// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"time"
)

func (s *Server) handleMessage(req *Message) (rsp *Message) {

	now := time.Now().UTC()
	s.lastActivity = now

	var dedup *dedupEntry
	isDup := false

	logDebug(req, nil, "received message")
	defer func() {
		if rsp != nil {

			if dedup != nil && !isDup {
				dedup.save(rsp)
			}

			rsp.Meta = req.Meta
			rsp.Meta.ReceivedAt = now
			logDebug(rsp, nil, "sent reply")
		}
	}()

	if s.dtlsListener != nil && req.Meta.ListenerName != s.dtlsListener.name {
		s.dtlsListener.ClosePeer(req.Meta.RemoteAddr)
	}

	if req.Type == TypeReset {
		logDebug(req, nil, "reset message received")
		return
	}

	if req.Type == TypeConfirmable || req.Type == TypeNonConfirmable {
		var ok bool
		dedup, ok = s.deduplicate(req)
		if !ok {
			if dedup.pending {
				logDebug(req, nil, "duplicate message, ignoring waiting on response")
				return
			}
			logDebug(req, nil, "duplicate message, cached response returned")
			rsp = dedup.rsp
			isDup = true
			return
		}
	}
	block1 := req.GetBlock1()
	if block1 != nil && req.IsRequest() {
		if block1.Num == 0 && !block1.More {
			// do nothing
		} else if block1.Num == 0 {
			// init waiter
			rsp = req.MakeReply(RspCodeContinue, nil)
			rsp.WithBlock1(block1)
			s.blockCachePut(req, "")
			return
		} else if !block1.More {
			// reassemble data
			var err error
			trsp, err := s.blockCacheGet(req, -1, 0)
			if err != nil {
				logError(req, err, "coap: error retrieving block1 cache")
				rsp = req.MakeReply(RspCodeInternalServerError, nil)
				return
			}
			req.Payload = trsp.Payload
		} else {
			// append data
			err := s.blockCacheAppend(req, block1)
			if err != nil {
				logError(req, err, "coap: error appending block1 cache")
				rsp = req.MakeReply(RspCodeInternalServerError, nil)
				return
			}
			rsp = req.MakeReply(RspCodeContinue, nil)
			rsp.WithBlock1(block1)
			return
		}
	}

	if req.Type == TypeAcknowledgement {
		found := s.handleAcknowledgement(req)
		if found {
			return
		}
	}

	block2 := req.GetBlock2()
	if block2 != nil {
		if req.IsRequest() {
			var err error
			rsp, err = s.blockCacheGet(req, block2.Num, block2.Size)
			if err == nil {
				return
			}
			/*
				if block2.Num == 0 {
					// fallthrough to main handler
				} else {
					// we have already processed block0 so now we need to feed the rest of the blocks.
					var err error
					rsp, err = s.blockCacheGet(req, block2.Num, block2.Size)
					if err != nil {
						logError(req, err, "coap: error getting block2 response")
						rsp = req.MakeReply(RspCodeInternalServerError, nil)
						return
					}
					return
				}*/
		} else if block2.More && block2.Num == 0 {
			// special case for notifications from observes that require blockwise
			rsp = req.MakeReply(CodeEmpty, nil)
			rsp.Token = nil
			_, err := s.send(req.Meta.RemoteAddr, rsp, s.NewOptions())
			if err != nil {
				logError(req, err, "coap: error getting failed to send empty ack to start block2 transfer")
			}
			rsp = nil
			// force query
			breq, err := s.blockRetreive(req)
			if err != nil {
				rsp = &Message{
					Type:      TypeReset,
					Code:      req.Code,
					MessageID: req.MessageID,
					Token:     req.Token,
				}
			} else {
				req = breq
			}
			block2.More = false
		}
	}

	if block2 != nil {
		req.Meta.BlockSize = block2.Size
	} else if block1 != nil {
		req.Meta.BlockSize = block1.Size
	}
	switch req.Type {
	case TypeConfirmable:
		if !req.IsRequest() {
			if req.Option(OptObserve) != nil {
				rsp = s.handleNotify(req)
			} else {
				if s.handleAcknowledgement(req) {
					rsp = req.MakeReply(CodeEmpty, nil)
					rsp.Meta.BlockSize = 0
					rsp.Token = nil
				}
			}
		} else {
			rsp = s.handleConfirmable(req)
		}
	case TypeNonConfirmable:
		if !req.IsRequest() {
			rsp = s.handleNotify(req)
		} else {
			rsp = s.handleConfirmable(req)
			if rsp.Type == TypeAcknowledgement {
				rsp.Type = TypeNonConfirmable
			}
		}
	case TypeAcknowledgement:
		rsp = s.handleNotify(req)
	default:
		rsp = &Message{
			Type:      TypeReset,
			Code:      req.Code,
			MessageID: req.MessageID,
			Token:     req.Token,
		}
	}

	if rsp != nil {
		bs := s.config.BlockDefaultSize
		if rsp.Meta.BlockSize != 0 {
			bs = rsp.Meta.BlockSize
		} else if req.Meta.BlockSize != 0 {
			bs = req.Meta.BlockSize
		}

		if block1 != nil {
			rsp.WithBlock1(block1)
		}

		if rsp.RequiresBlockwise() {
			//need to send BLOCK2
			if block2 == nil {
				block2 = blockInit(0, true, bs)
			} else {
				block2.More = true
				block2.Size = bs
			}

			//store request in block cache
			s.blockCachePut(rsp, req.getBlockKey())
			//rewrite rsp to include block0
			var err error
			rsp, err = s.blockCacheGet(req, block2.Num, bs)
			if err != nil {
				logError(req, err, "coap: error getting first block2")
				rsp = req.MakeReply(RspCodeInternalServerError, nil)
				return
			}
		}
	}

	return
}
