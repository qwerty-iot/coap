// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"time"
)

func Send(addr string, msg *Message, options *SendOptions) (*Message, error) {
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

	peer := dtlsFindPeer(addr)
	if peer != nil {
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
					peer := dtlsFindPeer(addr)
					if peer != nil {
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
