// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

type pendingEntry struct {
	c chan *Message
}

func (s *Server) pendingSave(msg *Message) chan *Message {
	pe := &pendingEntry{}
	pe.c = make(chan *Message, 1)
	if len(msg.Token) == 0 {
		msg.Token = []byte(randomString(8))
	}
	s.pendingMux.Lock()
	msg.MessageID = s.pendingMsgId
	s.pendingMsgId = s.pendingMsgId + 1
	s.pendingMap[string(msg.Token)] = pe
	s.pendingMidMap[msg.MessageID] = pe
	s.pendingMux.Unlock()
	return pe.c
}

func (s *Server) handleAcknowledgement(req *Message) bool {

	if req.Code == CodeEmpty {
		// delayed response
		s.pendingMux.Lock()
		pe, found := s.pendingMidMap[req.MessageID]
		if found {
			delete(s.pendingMidMap, req.MessageID)
		}
		s.pendingMux.Unlock()

		if !found {
			return false
		}

		select {
		case pe.c <- req:
			//logDebug(req, nil, "ack found (removed from pending list)")
		default:
			logDebug(req, nil, "empty ack on closed channel (removed from pending list)")
		}
		//logDebug(req, nil, "empty ack")
		return true
	}

	s.pendingMux.Lock()
	pe, found := s.pendingMap[string(req.Token)]
	if found {
		delete(s.pendingMap, string(req.Token))
		delete(s.pendingMidMap, req.MessageID)
	}
	s.pendingMux.Unlock()

	if found {
		select {
		case pe.c <- req:
			//logDebug(req, nil, "ack found (removed from pending list)")
		default:
			logDebug(req, nil, "ack on closed channel (removed from pending list)")
		}
		return true
	}
	//logDebug(req, nil, "ack not found")
	return false
}
