// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"errors"
	"math"
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
	msg.Meta.MaxMessageSize = options.MaxMessageSize

	if msg.RequiresBlockwise() {
		// chunk and send
		data := msg.Payload
		blockSize := msg.Meta.BlockSize
		blockNum := 0
		for {
			offset := blockNum * blockSize
			dataLen := blockSize
			more := true
			if offset+blockSize >= len(data) {
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

			msg.WithBlock1(nil)
			msg.Payload = nil

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

func (s *Server) GetNextMsgId() uint16 {
	s.pendingMux.Lock()
	nid := s.pendingMsgId
	s.pendingMsgId = s.pendingMsgId + 1
	s.pendingMux.Unlock()
	return nid
}

func extractProxyName(addr string) string {
	parts := strings.SplitN(addr, ":", 2)
	if len(parts) == 2 && strings.Contains(parts[0], ".") {
		// The prefix is not present, the first part is an IP address
		return ""
	}
	return parts[0]
}

func (s *Server) send(addr string, msg *Message, options *SendOptions) (*Message, error) {
	var pendingChan chan *Message

	msg.Meta.RemoteAddr = addr

	if msg.IsConfirmable() {
		nstrt := time.Now().UTC()
		nstartInc(addr, options.NStart)
		defer nstartDec(addr)
		pendingChan = s.pendingSave(msg)
		if time.Now().UTC().Sub(nstrt).Seconds() > 1.0 || nstartCount(addr, options.NStart) > 0 {
			logDebug(msg, nil, "nstart delay %.3fs (%d waiting)", time.Now().UTC().Sub(nstrt).Seconds(), nstartCount(addr, options.NStart))
		}
	} else if msg.MessageID == 0 {
		msg.MessageID = s.GetNextMsgId()
	}

	data, err := msg.marshalBinary()
	if err != nil {
		return nil, err
	}

	var peer *dtls.Peer
	if pxy := extractProxyName(addr); pxy != "" {
		msg.Meta.ListenerName = pxy
		err = proxyRecv(s, pxy, addr, data)
	} else if peer = s.dtlsListener.FindPeer(addr); peer != nil {
		msg.Meta.DtlsPeer = peer
		msg.Meta.ListenerName = s.dtlsListener.name
		err = peer.Write(data)
	} else if s.udpListener != nil {
		msg.Meta.ListenerName = s.udpListener.name
		err = s.udpListener.Send(addr, data)
	} else {
		err = errors.New("coap: no valid listener")
	}

	if err != nil {
		return nil, err
	}

	if msg.Type != TypeAcknowledgement && pendingChan != nil {
		maxWait := time.Duration(float64(float64(options.ActTimeout*time.Duration(math.Pow(2.0, float64(options.MaxRetransmit+1))-1)) * options.RandomFactor))
		timeout := time.Duration(((float64(options.ActTimeout)*options.RandomFactor)-float64(options.ActTimeout))*rand.Float64()) + options.ActTimeout
		logDebug(msg, err, "sent message (maxWait:%0.2fs timeout:%0.2fs maxRetransmit:%d)", maxWait.Seconds(), timeout.Seconds(), options.MaxRetransmit)
		if options.MaxRetransmit == -1 {
			select {
			case rsp := <-pendingChan:
				logDebug(rsp, err, "send ack'd (no retransmits)")
				return rsp, nil
			case <-time.After(maxWait):
				logDebug(msg, err, "send timeout (no retransmits)")
				return nil, ErrTimeout
			}
		} else {
			startTime := time.Now()
			for retryCount := 0; retryCount <= options.MaxRetransmit; retryCount++ {
				if retryCount == options.MaxRetransmit {
					timeout = maxWait - time.Now().Sub(startTime)
				}
				select {
				case rsp := <-pendingChan:
					if rsp.Code == CodeEmpty {
						if msg.IsRequest() {
							logDebug(rsp, err, "send received delayed ack'd (%0.2f seconds)", time.Since(startTime).Seconds())
							continue
						} else {
							logDebug(rsp, err, "send received empty ack (%0.2f seconds)", time.Since(startTime).Seconds())
							return rsp, nil
						}

					}
					logDebug(rsp, err, "send ack'd (%d transmits, %0.2f seconds)", retryCount+1, time.Since(startTime).Seconds())
					return rsp, nil
				case <-time.After(timeout):
					//retransmit
					if retryCount < options.MaxRetransmit {
						logDebug(msg, err, "send retry needed (%d/%d transmits, %0.2f seconds)", retryCount+1, options.MaxRetransmit+1, time.Since(startTime).Seconds())
						if pxy := extractProxyName(addr); pxy != "" {
							err = proxyRecv(s, pxy, addr, data)
						} else if peer != nil {
							err = peer.Write(data)
						} else if s.udpListener != nil {
							err = s.udpListener.Send(addr, data)
						} else {
							err = errors.New("coap: no valid listener")
						}
						timeout *= 2
						logDebug(msg, err, "resent message (timeout:%0.2fs)", timeout.Seconds())
						if err != nil {
							return nil, err
						}
					}
				}
			}
			logDebug(msg, err, "send ack timeout (%d transmits, %0.2f seconds)", options.MaxRetransmit+1, time.Since(startTime).Seconds())
			return nil, ErrTimeout
		}
	} else {
		logDebug(msg, err, "sent message (no reply expected)")
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
