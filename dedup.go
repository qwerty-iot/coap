// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"container/heap"
	"sync"
	"sync/atomic"
	"time"
)

type dedupEndpoint struct {
	mu      sync.Mutex
	entries sync.Map
	retired bool
	expiry  dedupExpiryQueue
}

type dedupEntry struct {
	expiresAt time.Time
	messageID uint16
	// A nil response means the original handler is still pending (or returned
	// no reply). Published responses are immutable cache-owned snapshots.
	rsp atomic.Pointer[Message]
}

// Empty keepalive Reset replies have no wire data beyond their message ID,
// which the entry already owns. A shared immutable marker avoids retaining a
// full Message for each one. Reply metadata is set from the current request by
// handleMessage, just as it is for full cached responses.
var dedupEmptyReset = &Message{Type: TypeReset, Code: CodeEmpty}

func (s *Server) deduplicate(msg *Message) (*dedupEntry, bool) {
	return s.deduplicateAt(msg, time.Now())
}

func (s *Server) deduplicateAt(msg *Message, now time.Time) (*dedupEntry, bool) {
	peerKey := msg.Meta.GetPeerKey()
	for {
		epI, ok := s.dedupMap.Load(peerKey)
		if !ok {
			epI, _ = s.dedupMap.LoadOrStore(peerKey, &dedupEndpoint{})
		}
		ep := epI.(*dedupEndpoint)
		ep.mu.Lock()
		if ep.retired {
			ep.mu.Unlock()
			continue
		}
		entryI, found := ep.entries.Load(msg.MessageID)
		if found {
			entry := entryI.(*dedupEntry)
			if now.Before(entry.expiresAt) {
				ep.mu.Unlock()
				return entry, false
			}
		}
		entry := &dedupEntry{expiresAt: now.Add(s.config.DeduplicateExpiration), messageID: msg.MessageID}
		ep.entries.Store(msg.MessageID, entry)
		heap.Push(&ep.expiry, entry)
		ep.mu.Unlock()
		return entry, true
	}
}

func (entry *dedupEntry) save(rsp *Message) {
	if rsp != nil && rsp.Type == TypeReset && rsp.Code == CodeEmpty && rsp.MessageID == entry.messageID &&
		len(rsp.Token) == 0 && len(rsp.Payload) == 0 && len(rsp.opts) == 0 &&
		rsp.packetSize == 0 && len(rsp.queryVars) == 0 && len(rsp.PathVars) == 0 {
		entry.rsp.Store(dedupEmptyReset)
		return
	}
	entry.rsp.Store(copyDedupResponse(rsp))
}

func (entry *dedupEntry) response() *Message {
	rsp := entry.rsp.Load()
	if rsp == dedupEmptyReset {
		return &Message{Type: TypeReset, Code: CodeEmpty, MessageID: entry.messageID}
	}
	return copyDedupResponse(rsp)
}

func copyDedupResponse(rsp *Message) *Message {
	if rsp == nil {
		return nil
	}
	copy := *rsp
	// marshalBinary sorts options in place. Metadata is also rewritten for each
	// send. Payload and token bytes are only read by the response send path.
	copy.opts = append(options(nil), rsp.opts...)
	return &copy
}

func (s *Server) expireDedupAt(now time.Time) {
	s.dedupMap.Range(func(key, value interface{}) bool {
		ep := value.(*dedupEndpoint)
		ep.mu.Lock()
		defer ep.mu.Unlock()
		if ep.retired {
			return true
		}
		for len(ep.expiry) > 0 && !now.Before(ep.expiry[0].expiresAt) {
			entry := heap.Pop(&ep.expiry).(*dedupEntry)
			// A lookup may already have replaced this expired message ID.
			ep.entries.CompareAndDelete(entry.messageID, entry)
		}
		if len(ep.expiry) == 0 {
			// Requests that already loaded this peer retry rather than inserting
			// into a cache which is no longer reachable from the server.
			ep.retired = true
			s.dedupMap.CompareAndDelete(key, ep)
		}
		return true
	})
}

// Expiry order is independent of admission lock order: concurrent readers may
// obtain their timestamps in a different order from acquiring the peer lock.
// The queue avoids rescanning every live message on each watcher tick.
type dedupExpiryQueue []*dedupEntry

func (q dedupExpiryQueue) Len() int                { return len(q) }
func (q dedupExpiryQueue) Less(i, j int) bool      { return q[i].expiresAt.Before(q[j].expiresAt) }
func (q dedupExpiryQueue) Swap(i, j int)           { q[i], q[j] = q[j], q[i] }
func (q *dedupExpiryQueue) Push(value interface{}) { *q = append(*q, value.(*dedupEntry)) }
func (q *dedupExpiryQueue) Pop() interface{} {
	last := len(*q) - 1
	entry := (*q)[last]
	(*q)[last] = nil // Do not retain expired replies in unused slice capacity.
	*q = (*q)[:last]
	return entry
}

func (s *Server) dedupWatcher() {
	for {
		time.Sleep(time.Second)
		s.expireDedupAt(time.Now())
	}
}
