// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"sync"
	"time"
)

type dedupEndpoint struct {
	entries sync.Map
}

type dedupEntry struct {
	pending bool
	rsp     *Message
}

func (s *Server) deduplicate(msg *Message) (*dedupEntry, bool) {
	epI, ok := s.dedupMap.Load(msg.Meta.RemoteAddr)
	if !ok {
		epI, _ = s.dedupMap.LoadOrStore(msg.Meta.RemoteAddr, &dedupEndpoint{})
	}
	ep := epI.(*dedupEndpoint)

	entryI, found := ep.entries.Load(msg.MessageID)
	if found {
		return entryI.(*dedupEntry), false
	}

	s.dedupDeleteAfter.Store(msg.Meta.RemoteAddr, msg.Meta.ReceivedAt.Add(s.config.DeduplicateExpiration))

	entry := &dedupEntry{pending: true}
	ep.entries.Store(msg.MessageID, entry)

	return entry, true
}

func (entry *dedupEntry) save(rsp *Message) {
	entry.rsp = rsp
	entry.pending = false
}

func (s *Server) dedupWatcher() {
	for {
		time.Sleep(time.Second)
		now := time.Now()
		s.dedupDeleteAfter.Range(func(key, value interface{}) bool {
			expirationTime := value.(time.Time)
			if expirationTime.Before(now) {
				s.dedupMap.Delete(key)
				s.dedupDeleteAfter.Delete(key)
			}
			return true
		})
	}
}
