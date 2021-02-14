// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import "sync"

type pendingEntry struct {
	c chan *Message
}

var pendingMap = map[string]*pendingEntry{}
var pendingMux sync.Mutex
var pendingMsgId uint16 = 1

func pendingSave(msg *Message) chan *Message {
	pe := &pendingEntry{}
	pe.c = make(chan *Message, 1)
	if len(msg.Token) == 0 {
		msg.Token = []byte(randomString(8))
	}
	pendingMux.Lock()
	msg.MessageID = pendingMsgId
	pendingMsgId = pendingMsgId + 1
	pendingMap[string(msg.Token)] = pe
	pendingMux.Unlock()
	return pe.c
}

func handleAcknowledgement(req *Message) bool {
	pendingMux.Lock()
	pe, found := pendingMap[string(req.Token)]
	delete(pendingMap, string(req.Token))
	pendingMux.Unlock()

	if found {
		select {
		case pe.c <- req:
		default:
		}
		return true
	}
	return false
}
