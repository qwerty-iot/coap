// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"errors"
	"strings"
	"time"

	"github.com/qwerty-iot/dtls/v2"
)

func Send(addr string, msg *Message, options *SendOptions) (*Message, error) {
	var rsp *Message
	var err error

	if msg.RequiresBlockwise() {
		// chunk and send
		data := msg.Payload
		blockSize := config.BlockDefaultSize
		if msg.Meta.BlockSize != 0 {
			blockSize = msg.Meta.BlockSize
		}
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
			rsp, err = send(addr, msg, options)
			if err != nil {
				return nil, err
			}
			if more && rsp.Code != RspCodeContinue {
				return nil, errors.New("expected block transfer continue response")
			}
			block1 := rsp.getBlock1()
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
		rsp, err = send(addr, msg, options)
		if err != nil {
			return nil, err
		}
	}

	block2 := rsp.getBlock2()
	if block2 != nil {
		//blockwise requests
		var data []byte
		data = append(data, rsp.Payload...)

		block := 1
		for {
			bm := blockInit(block, false, block2.Size)
			msg.WithBlock2(bm)
			rsp, err = send(addr, msg, options)
			if err != nil {
				return nil, err
			}
			data = append(data, rsp.Payload...)
			block++
			block2 = rsp.getBlock2()
			if !block2.More {
				break
			}
		}
		rsp.Payload = data
	}
	return rsp, err
}

func send(addr string, msg *Message, options *SendOptions) (*Message, error) {
	if options == nil {
		options = NewOptions()
	}
	var pendingChan chan *Message
	if msg.Type != TypeAcknowledgement {
		pendingChan = pendingSave(msg)
	}

	data, err := msg.marshalBinary()
	if err != nil {
		return nil, err
	}

	var peer *dtls.Peer
	if strings.HasPrefix(addr, "proxy:") {
		err = proxyRecv(addr, data)
	} else if peer = dtlsFindPeer(addr); peer != nil {
		err = peer.Write(data)
	} else {
		err = udpSend(addr, data)
	}
	if err != nil {
		return nil, err
	}

	if msg.Type != TypeAcknowledgement && pendingChan != nil {
		if options.retryCount == -1 {
			select {
			case rsp := <-pendingChan:
				return rsp, nil
			case <-time.After(options.retryTimeout):
				return nil, ErrTimeout
			}
		} else {
			for retryCount := 0; retryCount < options.retryCount; retryCount++ {
				select {
				case rsp := <-pendingChan:
					return rsp, nil
				case <-time.After(options.retryTimeout):
					//retransmit
					if strings.HasPrefix(addr, "proxy:") {
						err = proxyRecv(addr, data)
					} else if peer != nil {
						err = peer.Write(data)
					} else {
						err = udpSend(addr, data)
					}
					if err != nil {
						return nil, err
					}
				}
			}
			return nil, ErrTimeout
		}
	} else {
		return nil, nil
	}
}
