// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

func handleConfirmable(req *Message) *Message {
	var rsp *Message

	if req.Code == 0 {
		rsp = &Message{
			Type:      TypeReset,
			Code:      0,
			MessageID: req.MessageID,
			Token:     req.Token,
		}
		return rsp
	}

	callback := matchRoutes(req)
	if callback != nil {
		rsp = callback(req)
	} else {
		rsp = &Message{
			Type:      TypeAcknowledgement,
			Code:      RspCodeNotFound,
			MessageID: req.MessageID,
			Token:     req.Token,
		}
	}

	return rsp
}
