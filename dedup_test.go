package coap

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newDedupTestServer() *Server {
	return &Server{config: NewConfig(), routes: make(map[string]*routeEntry)}
}

func dedupTestMessage(id uint16) *Message {
	return &Message{Type: TypeConfirmable, Code: CodeEmpty, MessageID: id,
		Meta: Metadata{RemoteAddr: "127.0.0.1:1234", PeerKey: "gateway:test", ListenerName: "gateway"}}
}

func dedupTestCounts(s *Server) (peers, entries int) {
	s.dedupMap.Range(func(_, value interface{}) bool {
		peers++
		value.(*dedupEndpoint).entries.Range(func(_, _ interface{}) bool { entries++; return true })
		return true
	})
	return
}

func TestDedupContinuousTrafficExpiresOldEntries(t *testing.T) {
	s := newDedupTestServer()
	start := time.Unix(1000, 0)
	// One new message per second, continuously, for six default lifetimes.
	window := int(s.config.DeduplicateExpiration / time.Second)
	for i := 0; i < 6*window; i++ {
		now := start.Add(time.Duration(i) * time.Second)
		msg := dedupTestMessage(uint16(i))
		entry, fresh := s.deduplicateAt(msg, now)
		if !fresh {
			t.Fatalf("message %d unexpectedly duplicated", i)
		}
		entry.save(msg.MakeReply(CodeEmpty, nil))
		s.expireDedupAt(now)
		_, count := dedupTestCounts(s)
		want := i + 1
		if want > window {
			want = window
		}
		if count != want {
			t.Fatalf("second %d: got %d entries, want %d", i, count, want)
		}
	}
	s.expireDedupAt(start.Add(time.Duration(7*window) * time.Second))
	if peers, entries := dedupTestCounts(s); peers != 0 || entries != 0 {
		t.Fatalf("idle cache retained %d peers and %d entries", peers, entries)
	}
}

func TestDedupExpiryBoundaryAndLateCompletion(t *testing.T) {
	s := newDedupTestServer()
	start := time.Unix(1000, 0)
	msg := dedupTestMessage(7)
	old, _ := s.deduplicateAt(msg, start)
	expires := start.Add(s.config.DeduplicateExpiration)
	for _, now := range []time.Time{start.Add(time.Second), expires.Add(-time.Nanosecond)} {
		got, fresh := s.deduplicateAt(msg, now)
		if fresh || got != old || !got.expiresAt.Equal(expires) {
			t.Fatal("duplicate extended or replaced the entry")
		}
	}
	replacement, fresh := s.deduplicateAt(msg, expires)
	if !fresh || replacement == old {
		t.Fatal("message ID was not reusable at exact expiry")
	}
	old.save(msg.MakeReply(RspCodeNotFound, nil))
	if replacement.response() != nil {
		t.Fatal("late completion published into replacement")
	}
	replacement.save(msg.MakeReply(RspCodeChanged, nil))
	s.expireDedupAt(expires)
	got, fresh := s.deduplicateAt(msg, expires)
	if fresh || got.response().Code != RspCodeChanged {
		t.Fatal("cleanup lost replacement response")
	}
	s.expireDedupAt(expires.Add(s.config.DeduplicateExpiration))
	if peers, _ := dedupTestCounts(s); peers != 0 {
		t.Fatal("cleanup did not expire at exact boundary")
	}
}

func TestDedupPendingAndNoReplyExpire(t *testing.T) {
	s := newDedupTestServer()
	calls := 0
	s.AddRoute("/", func(*Message) *Message { calls++; return nil })
	msg := dedupTestMessage(1)
	msg.Code = CodePost
	if s.handleMessage(msg) != nil || s.handleMessage(msg) != nil || calls != 1 {
		t.Fatal("no-reply duplicate repeated handler")
	}
	s.expireDedupAt(time.Now().Add(s.config.DeduplicateExpiration))
	if peers, _ := dedupTestCounts(s); peers != 0 {
		t.Fatal("no-reply entry retained")
	}
	start := time.Now()
	entry, _ := s.deduplicateAt(dedupTestMessage(2), start)
	if entry.response() != nil {
		t.Fatal("new entry was not pending")
	}
	s.expireDedupAt(start.Add(s.config.DeduplicateExpiration))
	if peers, _ := dedupTestCounts(s); peers != 0 {
		t.Fatal("pending entry retained")
	}
}

func TestDedupConcurrentAdmissionAndPublication(t *testing.T) {
	s := newDedupTestServer()
	start := time.Now()
	var freshCount atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entry, fresh := s.deduplicateAt(dedupTestMessage(1), start)
			if fresh {
				freshCount.Add(1)
				rsp := dedupTestMessage(1).MakeReply(RspCodeChanged, []byte("ok"))
				rsp.opts = options{{ID: OptURIQuery, Value: "q=1"}, {ID: OptURIPath, Value: "test"}}
				entry.save(rsp)
			}
			for j := 0; j < 20; j++ {
				if rsp := entry.response(); rsp != nil {
					rsp.Meta.RemoteAddr = "send-local"
					if rsp.Code != RspCodeChanged {
						t.Error("partially published response")
					}
					if _, err := rsp.marshalBinary(); err != nil {
						t.Error(err)
					}
				}
			}
		}()
	}
	wg.Wait()
	if freshCount.Load() != 1 {
		t.Fatalf("admitted %d handlers", freshCount.Load())
	}
	entry, _ := s.deduplicateAt(dedupTestMessage(1), start)
	if entry.response() == nil {
		t.Fatal("response was not published")
	}
}

func TestDedupCleanupRacingInsertion(t *testing.T) {
	for round := 0; round < 100; round++ {
		s := newDedupTestServer()
		now := time.Unix(1000, 0)
		s.deduplicateAt(dedupTestMessage(0), now.Add(-s.config.DeduplicateExpiration))
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 32; i++ {
			wg.Add(1)
			go func(id uint16) {
				defer wg.Done()
				<-start
				s.deduplicateAt(dedupTestMessage(id), now)
			}(uint16(i + 1))
		}
		wg.Add(1)
		go func() { defer wg.Done(); <-start; s.expireDedupAt(now) }()
		close(start)
		wg.Wait()
		for i := 1; i <= 32; i++ {
			if _, fresh := s.deduplicateAt(dedupTestMessage(uint16(i)), now); fresh {
				t.Fatalf("round %d: lost message %d", round, i)
			}
		}
		s.expireDedupAt(now.Add(s.config.DeduplicateExpiration))
		if peers, _ := dedupTestCounts(s); peers != 0 {
			t.Fatal("empty peer retained")
		}
	}
}

func TestDedupPeerIsolationWithExpiry(t *testing.T) {
	s := newDedupTestServer()
	now := time.Now()
	first, second := dedupTestMessage(1), dedupTestMessage(1)
	second.Meta.PeerKey = "gateway:other"
	s.deduplicateAt(first, now)
	secondEntry, fresh := s.deduplicateAt(second, now.Add(time.Second))
	if !fresh {
		t.Fatal("scoped peers collided")
	}
	s.expireDedupAt(now.Add(s.config.DeduplicateExpiration))
	if got, fresh := s.deduplicateAt(second, now.Add(s.config.DeduplicateExpiration)); fresh || got != secondEntry {
		t.Fatal("expiry affected another peer")
	}
}

func TestDedupExpiryQueueHandlesOutOfOrderAdmission(t *testing.T) {
	s := newDedupTestServer()
	now := time.Now()
	// A reader can take its timestamp before another reader, but acquire the
	// peer lock later. Cleanup must still visit the earlier expiry first.
	later, _ := s.deduplicateAt(dedupTestMessage(1), now.Add(time.Second))
	s.deduplicateAt(dedupTestMessage(2), now)
	s.expireDedupAt(now.Add(s.config.DeduplicateExpiration))
	if _, count := dedupTestCounts(s); count != 1 {
		t.Fatalf("out-of-order expiry retained %d entries, want 1", count)
	}
	got, fresh := s.deduplicateAt(dedupTestMessage(1), now.Add(s.config.DeduplicateExpiration))
	if fresh || got != later {
		t.Fatal("expired the wrong entry")
	}
	s.expireDedupAt(now.Add(time.Second + s.config.DeduplicateExpiration))
	if peers, _ := dedupTestCounts(s); peers != 0 {
		t.Fatal("last peer retained")
	}
}

func TestDedupResponseCopies(t *testing.T) {
	entry := &dedupEntry{}
	rsp := dedupTestMessage(1).MakeReply(RspCodeChanged, []byte("payload"))
	rsp.opts = options{{ID: OptURIQuery, Value: "q=1"}, {ID: OptURIPath, Value: "test"}}
	rsp.Meta.RemoteAddr = "original"
	entry.save(rsp)
	rsp.Meta.RemoteAddr = "modified"
	rsp.opts[0].Value = "modified"
	first := entry.response()
	if first.Meta.RemoteAddr != "original" || first.opts[0].Value != "q=1" {
		t.Fatal("cache shares mutable response state")
	}
	first.Meta.RemoteAddr = "duplicate"
	if _, err := first.marshalBinary(); err != nil {
		t.Fatal(err)
	}
	second := entry.response()
	if second.Meta.RemoteAddr != "original" || second.opts[0].ID != OptURIQuery {
		t.Fatal("duplicate changed cached metadata/options")
	}
}

func TestDedupEmptyResetRepresentation(t *testing.T) {
	for _, kind := range []string{"empty", "token", "payload", "options", "packet-size", "path-vars", "query-vars", "different-id"} {
		t.Run(kind, func(t *testing.T) {
			entry := &dedupEntry{messageID: 42}
			rsp := &Message{Type: TypeReset, Code: CodeEmpty, MessageID: 42}
			switch kind {
			case "token":
				rsp.Token = []byte{1}
			case "payload":
				rsp.Payload = []byte("payload")
			case "options":
				rsp.opts = options{{ID: OptURIPath, Value: "test"}}
			case "packet-size":
				rsp.packetSize = 4
			case "path-vars":
				rsp.PathVars = map[string]string{"key": "value"}
			case "query-vars":
				rsp.queryVars = map[string]string{"key": "value"}
			case "different-id":
				rsp.MessageID = 43
			}
			entry.save(rsp)
			if compact := entry.rsp.Load() == dedupEmptyReset; compact != (kind == "empty") {
				t.Fatalf("unexpected compact representation: %v", compact)
			}
			original, err := rsp.marshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			got := entry.response()
			wire, err := got.marshalBinary()
			if err != nil || !bytes.Equal(wire, original) {
				t.Fatalf("reply changed: %x / %x (%v)", wire, original, err)
			}
			got.MessageID = 99
			if entry.response().MessageID != rsp.MessageID {
				t.Fatal("replay mutated cached Reset")
			}
		})
	}
}

func TestDedupDuplicateReplyWireBehavior(t *testing.T) {
	for _, kind := range []string{"keepalive", "normal", "blockwise"} {
		t.Run(kind, func(t *testing.T) {
			s := newDedupTestServer()
			calls := 0
			s.AddRoute("~keepalive", func(*Message) *Message { calls++; return nil })
			s.AddRoute("/", func(req *Message) *Message {
				calls++
				payload := []byte("normal reply")
				if kind == "blockwise" {
					payload = bytes.Repeat([]byte("x"), 2048)
				}
				rsp := req.MakeReply(RspCodeContent, payload)
				if kind == "blockwise" {
					rsp.Meta.BlockSize = 1024
				}
				return rsp
			})
			msg := dedupTestMessage(42)
			if kind != "keepalive" {
				msg.Code = CodeGet
				msg.Token = []byte{1, 2}
			}
			first := s.handleMessage(msg)
			if first == nil {
				t.Fatal("missing original reply")
			}
			wire, err := first.marshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			if kind == "keepalive" && !bytes.Equal(wire, []byte{0x70, 0, 0, 42}) {
				t.Fatalf("unexpected Reset: %x", wire)
			}
			if kind == "normal" && (first.Code != RspCodeContent || string(first.Payload) != "normal reply") {
				t.Fatal("normal reply changed")
			}
			if kind == "blockwise" {
				block := first.GetBlock2()
				if block == nil || block.Num != 0 || !block.More || len(first.Payload) != 1024 {
					t.Fatal("first block changed")
				}
			}
			for i := 0; i < 3; i++ {
				dupReq := *msg
				dupReq.Meta.RemoteAddr = "127.0.0.1:4321"
				dup := s.handleMessage(&dupReq)
				if dup == nil || dup == first {
					t.Fatal("missing or shared duplicate response")
				}
				got, err := dup.marshalBinary()
				if err != nil || !bytes.Equal(got, wire) {
					t.Fatalf("duplicate wire differs: %x / %x (%v)", got, wire, err)
				}
				if dup.Meta.GetPeerKey() != msg.Meta.GetPeerKey() {
					t.Fatal("peer identity changed")
				}
				if dup.Meta.RemoteAddr != dupReq.Meta.RemoteAddr || dup.Meta.ReceivedAt.Location() != time.UTC {
					t.Fatal("duplicate did not receive current request metadata in UTC")
				}
			}
			if calls != 1 {
				t.Fatalf("handler called %d times", calls)
			}
		})
	}
}

func BenchmarkDedupActivePeers(b *testing.B) {
	s := newDedupTestServer()
	start := time.Now()
	peers := make([]string, 64)
	for i := range peers {
		peers[i] = fmt.Sprintf("peer-%d", i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		now := start.Add(time.Duration(i/64) * time.Second)
		msg := dedupTestMessage(uint16(i / 64))
		msg.Meta.PeerKey = peers[i%64]
		entry, fresh := s.deduplicateAt(msg, now)
		if fresh {
			entry.save(s.handleConfirmable(msg))
		}
		if i%64 == 0 {
			s.expireDedupAt(now)
		}
	}
}
