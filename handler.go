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

	logDebug(req, nil, "received message")
	defer func() {
		if rsp != nil {
			rsp.Meta = req.Meta
			rsp.Meta.ReceivedAt = now
			logDebug(rsp, nil, "sent reply")
		}
	}()

	if s.dtlsListener != nil && req.Meta.ListenerName != s.dtlsListener.name {
		s.dtlsListener.ClosePeer(req.Meta.RemoteAddr)
	}

	var dedup *dedupEntry
	if req.Type == TypeConfirmable {
		var ok bool
		dedup, ok = s.deduplicate(req)
		if !ok {
			if dedup.pending {
				logDebug(req, nil, "duplicate message, ignoring waiting on response")
				return
			}
			logDebug(req, nil, "duplicate message, cached response returned")
			rsp = dedup.rsp
			return
		}
	}
	block1 := req.GetBlock1()
	if block1 != nil && req.Type != TypeAcknowledgement {
		if block1.Num == 0 {
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
			err := s.blockCacheAppend(req)
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
			}
		} else if block2.More && block2.Num == 0 {
			// special case for notifications from observes that require blockwise
			rsp = req.MakeReply(CodeEmpty, nil)
			rsp.Token = nil
			s.send(req.Meta.RemoteAddr, rsp, s.NewOptions())
			rsp = nil
			// force query
			var err error
			req, err = s.blockRetreive(req)
			if err != nil {
				rsp = &Message{
					Type:      TypeReset,
					Code:      req.Code,
					MessageID: req.MessageID,
					Token:     req.Token,
				}
			}
			block2.More = false
		}
	}

	if block2 != nil {
		req.Meta.BlockSize = block2.Size
	}
	switch req.Type {
	case TypeConfirmable:
		if req.Option(OptObserve) != nil && !req.IsRequest() {
			rsp = s.handleNotify(req)
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
		if block1 != nil {
			rsp.WithBlock1(block1)
		}
		if rsp.RequiresBlockwise() {
			//need to send BLOCK2
			bs := s.config.BlockDefaultSize
			if rsp.Meta.BlockSize != 0 {
				bs = rsp.Meta.BlockSize
			}
			block2 = blockInit(0, true, bs)

			//store request in block cache
			s.blockCachePut(rsp, req.getBlockKey())
			//rewrite rsp to include block0
			var err error
			rsp, err = s.blockCacheGet(req, 0, bs)
			if err != nil {
				logError(req, err, "coap: error getting first block2")
				rsp = req.MakeReply(RspCodeInternalServerError, nil)
				return
			}
		}
	}

	if dedup != nil {
		dedup.save(rsp)
	}
	return
}
