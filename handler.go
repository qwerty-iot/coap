// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

func handleMessage(req *Message) *Message {
	var rsp *Message

	var dedup *dedupEntry
	if req.Type == TypeConfirmable {
		var ok bool
		dedup, ok = deduplicate(req)
		if !ok {
			if dedup.pending {
				return nil
			}
			return dedup.rsp
		}
	}

	switch req.Type {
	case TypeConfirmable:
		if req.Option(OptObserve) != nil {
			rsp = handleNotify(req)
		} else {
			rsp = handleConfirmable(req)
		}
	case TypeNonConfirmable:
		rsp = handleNotify(req)
	case TypeAcknowledgement:
		found := handleAcknowledgement(req)
		if !found {
			//note, we won't send a reset on a bad notify in this case
			handleNotify(req)
		}
	default:
		rsp = &Message{
			Type:      TypeReset,
			Code:      req.Code,
			MessageID: req.MessageID,
			Token:     req.Token,
		}
	}

	if dedup != nil {
		dedup.save(rsp)
	}
	return rsp
}
