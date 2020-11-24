package coap

import (
	"sync"
	"time"
)

var dedupMap sync.Map
var dedupDeleteAfter sync.Map

type dedupEndpoint struct {
	entries sync.Map
}

type dedupEntry struct {
	pending  bool
	rsp *Message
}

func deduplicate(msg *Message) (*dedupEntry, bool) {
	epI, ok := dedupMap.Load(msg.Meta.RemoteAddr)
	if !ok {
		epI, _ = dedupMap.LoadOrStore(msg.Meta.RemoteAddr, &dedupEndpoint{})
	}
	ep := epI.(*dedupEndpoint)

	dedupDeleteAfter.Store(msg.Meta.RemoteAddr, msg.Meta.ReceivedAt.Add(config.DeduplicateExpiration))

	entryI, found := ep.entries.Load(msg.MessageID)
	if found {
		return entryI.(*dedupEntry), false
	}

	entry := &dedupEntry{pending: true}
	ep.entries.Store(msg.MessageID, entry)

	return entry, true
}

func (entry *dedupEntry) save(rsp *Message) {
	entry.rsp = rsp
	entry.pending = false
}

func dedupWatcher() {
	for {
		select {
		case <-time.After(time.Second):
			now := time.Now()
			dedupDeleteAfter.Range(func(key, value interface{}) bool {
				expirationTime := value.(time.Time)
				if expirationTime.Before(now) {
					dedupMap.Delete(key)
					dedupDeleteAfter.Delete(key)
				}
				return true
			})
		}
	}
}
