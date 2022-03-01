// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import "time"

func (s *Server) handleMessage(req *Message) (rsp *Message) {

	logDebug(req, nil, "received message")
	defer func() {
		if rsp != nil {
			rsp.Meta = req.Meta
			rsp.Meta.ReceivedAt = time.Now().UTC()
			logDebug(rsp, nil, "sent reply")
		}
	}()

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
		if !found {
			//note, we won't send a reset on a bad notify in this case
			s.handleNotify(req)
		}
		return
	}

	block2 := req.GetBlock2()

	if block2 == nil || block2.Num == 0 && req.Type == TypeConfirmable || req.Type != TypeConfirmable {
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
	} else {
		// retrieve message
		var err error
		rsp, err = s.blockCacheGet(req, block2.Num, block2.Size)
		if err != nil {
			logError(req, err, "coap: error getting block2 response")
			rsp = req.MakeReply(RspCodeInternalServerError, nil)
			return
		}
	}

	if dedup != nil {
		dedup.save(rsp)
	}
	return
}
