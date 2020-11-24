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
	pendingMap[string(msg.Token)] = pe
	pendingMsgId = pendingMsgId + 1
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