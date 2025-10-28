// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

func (s *Server) handleConfirmable(req *Message) *Message {
	var rsp *Message

	if req.Code == CodeEmpty {
		callback := s.getSpecialRoute("~keepalive")
		if callback != nil {
			callback(req)
		}
		rsp = &Message{
			Type:      TypeReset,
			Code:      0,
			MessageID: req.MessageID,
			Token:     req.Token,
		}
		return rsp
	}

	if req.Code > 10 {
		// delayed ack
		if s.handleAcknowledgement(req) {
			rsp = req.MakeReply(CodeEmpty, nil)
		} else {
			rsp = req.MakeReply(RspCodeNotFound, nil)
		}
	} else {
		callback := s.matchRoutes(req)
		if callback != nil {
			rsp = callback(req)
		} else {
			rsp = req.MakeReply(RspCodeNotFound, nil)
		}
	}

	return rsp
}
