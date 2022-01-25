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

	msg.Meta.BlockSize = options.blockSize

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
			blockNum = block1.Num
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
		if block2 != nil {
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
		nstartInc(addr, options.nStart)
		defer nstartDec(addr)
		pendingChan = s.pendingSave(msg)
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
		timeout := options.ackTimeout + time.Second*time.Duration(options.ackTimeout.Seconds()*((options.randomFactor-1.0)*rand.Float64()))
		if options.maxRetransmit == -1 {
			select {
			case rsp := <-pendingChan:
				logDebug(rsp, err, "send ack'd (no retries)")
				return rsp, nil
			case <-time.After(timeout):
				logDebug(msg, err, "send ack timeout (no retries)")
				return nil, ErrTimeout
			}
		} else {
			for retryCount := 0; retryCount < options.maxRetransmit; retryCount++ {
				select {
				case rsp := <-pendingChan:
					logDebug(rsp, err, "send ack'd (%d retries)", retryCount)
					return rsp, nil
				case <-time.After(timeout):
					//retransmit
					logDebug(msg, err, "send ack timeout (%d/%d retries)", retryCount, options.maxRetransmit)
					if strings.HasPrefix(addr, "proxy:") {
						err = proxyRecv(addr, data)
					} else if peer != nil {
						err = peer.Write(data)
					} else if s.udpListener != nil {
						err = s.udpListener.Send(addr, data)
					} else {
						err = errors.New("coap: no valid listener")
					}
					logDebug(msg, err, "sent message")
					if err != nil {
						return nil, err
					}
				}
			}
			logDebug(msg, err, "send ack timeout (%d retries)", options.maxRetransmit)
			return nil, ErrTimeout
		}
	} else {
		return nil, nil
	}
}
