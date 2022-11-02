// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"errors"
	"math/rand"
	"strings"
	"time"

	"github.com/qwerty-iot/dtls/v2"
)

func (s *Server) Send(addr string, msg *Message, options *SendOptions) (*Message, error) {
	var rsp *Message
	var err error

	msg.Meta.RemoteAddr = addr

	if options == nil {
		options = s.NewOptions()
	}

	msg.Meta.BlockSize = options.BlockSize

	if msg.RequiresBlockwise() {
		// chunk and send
		data := msg.Payload
		blockSize := msg.Meta.BlockSize
		blockNum := 0
		for {
			offset := blockNum * blockSize
			dataLen := blockSize
			more := true
			if offset+blockSize > len(data) {
				dataLen = len(data) - blockNum*blockSize
				more = false
			}
			msg.Payload = data[offset : offset+dataLen]
			msg.WithBlock1(blockInit(blockNum, more, blockSize))
			if blockNum == 0 {
				msg.WithSize1(len(data))
			}
			rsp, err = s.send(addr, msg, options)
			if err != nil {
				return nil, err
			}
			if more && rsp.Code != RspCodeContinue {
				return nil, errors.New("expected block transfer continue response")
			}
			block1 := rsp.GetBlock1()
			if block1 == nil {
				return nil, errors.New("expected block1 in response")
			}
			if blockSize > block1.Size {
				// size changed, need to adjust block num
				blockNum = (blockSize/block1.Size)*(block1.Num+1) - 1
			} else {
				blockNum = block1.Num
			}
			blockSize = block1.Size
			if !more {
				break
			}
			blockNum++
		}
	} else {
		rsp, err = s.send(addr, msg, options)
		if err != nil {
			return nil, err
		}
	}

	if rsp != nil {
		block2 := rsp.GetBlock2()
		if block2 != nil && block2.More {
			//blockwise requests
			var data []byte
			data = append(data, rsp.Payload...)

			block := 1
			for {
				bm := blockInit(block, false, block2.Size)
				msg.WithBlock2(bm)
				rsp, err = s.send(addr, msg, options)
				if err != nil {
					return nil, err
				}
				data = append(data, rsp.Payload...)
				block++
				block2 = rsp.GetBlock2()
				if !block2.More {
					break
				}
			}
			rsp.Payload = data
		}
	}

	return rsp, err
}

func (s *Server) send(addr string, msg *Message, options *SendOptions) (*Message, error) {
	var pendingChan chan *Message

	msg.Meta.RemoteAddr = addr

	if msg.IsConfirmable() {
		nstrt := time.Now().UTC()
		nstartInc(addr, options.NStart)
		if time.Now().UTC().Sub(nstrt).Seconds() > 0.5 || nstartCount(addr, options.NStart) > 0 {
			logDebug(msg, nil, "nstart delay %.3fms (%d waiting)", time.Now().UTC().Sub(nstrt).Seconds(), nstartCount(addr, options.NStart))
		}
		defer nstartDec(addr)
		pendingChan = s.pendingSave(msg)
	} else if msg.MessageID == 0 {
		s.pendingMux.Lock()
		msg.MessageID = s.pendingMsgId
		s.pendingMsgId = s.pendingMsgId + 1
		s.pendingMux.Unlock()
	}

	data, err := msg.marshalBinary()
	if err != nil {
		return nil, err
	}

	var peer *dtls.Peer
	if strings.HasPrefix(addr, "proxy:") {
		msg.Meta.ListenerName = "proxy"
		err = proxyRecv(addr, data)
	} else if peer = s.dtlsListener.FindPeer(addr); peer != nil {
		msg.Meta.DtlsIdentity = peer.SessionIdentityString()
		msg.Meta.DtlsCertificate = peer.SessionCertificate()
		msg.Meta.DtlsPublicKey = peer.SessionPublicKey()
		msg.Meta.ListenerName = s.dtlsListener.name
		err = peer.Write(data)
	} else if s.udpListener != nil {
		msg.Meta.ListenerName = s.udpListener.name
		err = s.udpListener.Send(addr, data)
	} else {
		err = errors.New("coap: no valid listener")
	}
	logDebug(msg, err, "sent message")
	if err != nil {
		return nil, err
	}

	if msg.Type != TypeAcknowledgement && pendingChan != nil {
		maxWait := time.Duration(float64(options.ActTimeout*time.Duration((2^(options.MaxRetransmit+1))-1)) * options.RandomFactor)
		timeout := time.Duration(((float64(options.ActTimeout)*options.RandomFactor)-float64(options.ActTimeout))*rand.Float64()) + options.ActTimeout
		if options.MaxRetransmit == -1 {
			select {
			case rsp := <-pendingChan:
				logDebug(rsp, err, "send ack'd (no retransmits)")
				return rsp, nil
			case <-time.After(maxWait):
				logDebug(msg, err, "send ack timeout (no retransmits)")
				return nil, ErrTimeout
			}
		} else {
			startTime := time.Now()
			for retryCount := 0; retryCount < options.MaxRetransmit; retryCount++ {
				if retryCount == options.MaxRetransmit-1 {
					timeout = maxWait - time.Now().Sub(startTime)
				}
				select {
				case rsp := <-pendingChan:
					logDebug(rsp, err, "send ack'd (%d transmits, %0.2f seconds)", retryCount+1, time.Since(startTime).Seconds())
					return rsp, nil
				case <-time.After(timeout):
					//retransmit
					logDebug(msg, err, "send ack timeout (%d/%d transmits, %0.2f seconds, will retry)", retryCount+1, options.MaxRetransmit, time.Since(startTime).Seconds())
					if strings.HasPrefix(addr, "proxy:") {
						err = proxyRecv(addr, data)
					} else if peer != nil {
						err = peer.Write(data)
					} else if s.udpListener != nil {
						err = s.udpListener.Send(addr, data)
					} else {
						err = errors.New("coap: no valid listener")
					}
					logDebug(msg, err, "resent message")
					if err != nil {
						return nil, err
					}
				}
				timeout *= 2
			}
			logDebug(msg, err, "send ack timeout (%d transmits, %0.2f seconds)", options.MaxRetransmit, time.Since(startTime).Seconds())
			return nil, ErrTimeout
		}
	} else {
		return nil, nil
	}
}

func (s *Server) blockRetreive(req *Message) (*Message, error) {

	obs := s.getObserve(req)
	if obs == nil {
		return nil, errors.New("expected observation to exist")
	}

	block2 := req.GetBlock2()
	if block2 == nil {
		return nil, errors.New("expected block2 in request")
	}

	//blockwise requests
	var data []byte
	data = append(data, req.Payload...)

	logDebug(req, nil, "retrieving additional blocks starting")

	block := 1
	code := CodeGet
	if obs.path == "" {
		code = CodeFetch
	}
	for {
		msg := NewMessage().WithPathString(obs.path).WithType(TypeConfirmable).WithCode(code)
		bm := blockInit(block, false, block2.Size)
		msg.WithBlock2(bm)
		if af := req.Accept(); af != None {
			msg.WithContentFormat(af)
		}
		rsp, err := s.send(req.Meta.RemoteAddr, msg, s.NewOptions())
		if err != nil {
			return nil, err
		}
		data = append(data, rsp.Payload...)
		block++
		block2 = rsp.GetBlock2()
		if block2 == nil || !block2.More {
			break
		}
	}
	req.Payload = data
	req.Type = TypeAcknowledgement
	logDebug(req, nil, "retrieving additional blocks done")
	return req, nil
}
