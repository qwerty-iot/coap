package coap

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Use a synchronous proxy transport to exercise the real send/pending paths
// without listener background workers or external devices.
func contextTestServer(t *testing.T, send func(*Server, []byte) error) (*Server, string) {
	t.Helper()
	socket, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = socket.Close() })
	s := &Server{config: NewConfig(), pendingMap: map[string]*pendingEntry{}, pendingMidMap: map[uint16]*pendingEntry{},
		udpListener: &UdpListener{socket: socket}}
	s.config.AddProxyReceiver("test", func(data []byte, _ string) error { return send(s, data) })
	key := "test:" + t.Name()
	t.Cleanup(func() { NstartClear(key) })
	return s, key
}

func testRequest() *Message { return NewMessage().WithType(TypeConfirmable).WithCode(CodePost) }

func assertSendError(t *testing.T, err error, phase SendPhase, cause error) {
	t.Helper()
	var se *SendError
	if !errors.As(err, &se) || se.Phase != phase || !errors.Is(err, cause) {
		t.Fatalf("got %v, want %s / %v", err, phase, cause)
	}
	if cause == context.DeadlineExceeded && !errors.Is(err, ErrTimeout) {
		t.Fatal("deadline lost timeout classification")
	}
}

func assertPendingEmpty(t *testing.T, s *Server) {
	t.Helper()
	s.pendingMux.Lock()
	defer s.pendingMux.Unlock()
	if len(s.pendingMap) != 0 || len(s.pendingMidMap) != 0 {
		t.Fatal("pending exchange retained")
	}
}

func ackPacket(t *testing.T, s *Server, raw []byte) {
	t.Helper()
	var req Message
	if err := req.unmarshalBinary(raw); err != nil {
		t.Error(err)
		return
	}
	s.handleAcknowledgement(&Message{Type: TypeAcknowledgement, Code: RspCodeChanged, MessageID: req.MessageID, Token: req.Token})
}

func TestSendContextQueuedDeadlineDoesNotSendOrReleaseHolder(t *testing.T) {
	var writes atomic.Int32
	s, key := contextTestServer(t, func(*Server, []byte) error { writes.Add(1); return nil })
	release, err := nstartAcquire(context.Background(), key, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = s.SendToPeerContext(ctx, key, "127.0.0.1:1", testRequest(), nil)
	assertSendError(t, err, SendPhaseNStart, context.DeadlineExceeded)
	if writes.Load() != 0 {
		t.Fatal("queued request transmitted")
	}
	probe, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if _, err := nstartAcquire(probe, key, 1, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("waiter released holder's slot")
	}
	assertPendingEmpty(t, s)
}

func TestSendContextExchangeDeadlineReleasesNextWaiter(t *testing.T) {
	firstSent := make(chan struct{})
	var writes atomic.Int32
	s, key := contextTestServer(t, func(s *Server, raw []byte) error {
		if writes.Add(1) == 1 {
			close(firstSent)
		} else {
			ackPacket(t, s, raw)
		}
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	first := make(chan error, 1)
	go func() { _, err := s.SendToPeerContext(ctx, key, "127.0.0.1:1", testRequest(), nil); first <- err }()
	<-firstSent
	nextCtx, nextCancel := context.WithTimeout(context.Background(), time.Second)
	defer nextCancel()
	_, err := s.SendToPeerContext(nextCtx, key, "127.0.0.1:1", testRequest(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertSendError(t, <-first, SendPhaseExchange, context.DeadlineExceeded)
	if writes.Load() != 2 {
		t.Fatalf("unexpected transmissions: %d", writes.Load())
	}
	assertPendingEmpty(t, s)
}

func TestSendContextAdmissionDoesNotDetachContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	s, key := contextTestServer(t, func(*Server, []byte) error { cancel(); return nil })
	release, _ := nstartAcquire(context.Background(), key, 1, 0)
	done := make(chan error, 1)
	go func() { _, err := s.SendToPeerContext(ctx, key, "127.0.0.1:1", testRequest(), nil); done <- err }()
	release()
	assertSendError(t, <-done, SendPhaseExchange, context.Canceled)
	assertPendingEmpty(t, s)
}

func TestSendContextBlockwiseUsesOriginalContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var writes int
	s, key := contextTestServer(t, func(s *Server, raw []byte) error {
		writes++
		if writes == 2 {
			cancel()
			return nil
		}
		var req Message
		if err := req.unmarshalBinary(raw); err != nil {
			return err
		}
		rsp := &Message{Type: TypeAcknowledgement, Code: RspCodeContinue, Token: req.Token, MessageID: req.MessageID}
		rsp.WithBlock1(req.GetBlock1())
		s.handleAcknowledgement(rsp)
		return nil
	})
	opts := s.NewOptions().WithBlockSize(16).WithMaxMessageSize(0)
	msg := testRequest()
	msg.Payload = make([]byte, 64)
	_, err := s.SendToPeerContext(ctx, key, "127.0.0.1:1", msg, opts)
	assertSendError(t, err, SendPhaseExchange, context.Canceled)
	if writes != 2 {
		t.Fatalf("sent %d blocks after cancellation", writes)
	}
	assertPendingEmpty(t, s)
}

func TestSendContextResponseBlocksShareDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var writes int
	s, key := contextTestServer(t, func(s *Server, raw []byte) error {
		writes++
		if writes != 1 {
			return nil // The next response block never arrives.
		}
		var req Message
		if err := req.unmarshalBinary(raw); err != nil {
			return err
		}
		rsp := &Message{Type: TypeAcknowledgement, Code: RspCodeContent, Token: req.Token, MessageID: req.MessageID}
		rsp.WithBlock2(blockInit(0, true, 16))
		s.handleAcknowledgement(rsp)
		return nil
	})
	_, err := s.SendToPeerContext(ctx, key, "127.0.0.1:1", testRequest(), nil)
	assertSendError(t, err, SendPhaseExchange, context.DeadlineExceeded)
	if writes != 2 {
		t.Fatalf("unexpected response block requests: %d", writes)
	}
	assertPendingEmpty(t, s)
}

func TestSendContextBlockValidationErrorHasPhase(t *testing.T) {
	s, key := contextTestServer(t, func(s *Server, raw []byte) error { ackPacket(t, s, raw); return nil })
	msg := testRequest()
	msg.Payload = make([]byte, 64)
	opts := s.NewOptions().WithBlockSize(16).WithMaxMessageSize(0)
	_, err := s.SendToPeerContext(context.Background(), key, "127.0.0.1:1", msg, opts)
	var se *SendError
	if !errors.As(err, &se) || se.Phase != SendPhaseExchange || errors.Is(err, ErrTimeout) {
		t.Fatalf("incorrect block validation error: %v", err)
	}
	assertPendingEmpty(t, s)
}

func TestSendContextACKCancellationRace(t *testing.T) {
	for i := 0; i < 50; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		var race sync.WaitGroup
		race.Add(2)
		s, key := contextTestServer(t, func(s *Server, raw []byte) error {
			go func() { defer race.Done(); ackPacket(t, s, raw) }()
			go func() { defer race.Done(); cancel() }()
			return nil
		})
		_, err := s.SendToPeerContext(ctx, key, "127.0.0.1:1", testRequest(), nil)
		race.Wait()
		cancel()
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		assertPendingEmpty(t, s)
		probe, stop := context.WithTimeout(context.Background(), time.Second)
		release, err := nstartAcquire(probe, key, 1, 0)
		stop()
		if err != nil {
			t.Fatal("slot leaked", err)
		}
		release()
	}
}

func TestPendingCleanupDoesNotRemoveReplacement(t *testing.T) {
	s := &Server{pendingMap: map[string]*pendingEntry{}, pendingMidMap: map[uint16]*pendingEntry{}}
	msg := testRequest().WithToken([]byte("same"))
	old := s.pendingSave(msg)
	mid := msg.MessageID
	s.pendingMsgId = mid
	newChan := s.pendingSave(msg)
	s.pendingRemove("same", mid, old)
	if s.pendingMap["same"].c != newChan || s.pendingMidMap[mid].c != newChan {
		t.Fatal("removed newer registration")
	}
	s.pendingRemove("same", mid, newChan)
	assertPendingEmpty(t, s)
}

func TestSendContextTransportFailureReleasesSlot(t *testing.T) {
	failure := errors.New("transport failure")
	s, key := contextTestServer(t, func(*Server, []byte) error { return failure })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := s.SendToPeerContext(ctx, key, "127.0.0.1:1", testRequest(), nil)
	assertSendError(t, err, SendPhaseExchange, failure)
	assertPendingEmpty(t, s)
	release, err := nstartAcquire(ctx, key, 1, 0)
	if err != nil {
		t.Fatal("slot leaked", err)
	}
	release()
}

func TestNstartCancellationAdmissionRace(t *testing.T) {
	key := t.Name()
	defer NstartClear(key)
	for i := 0; i < 50; i++ {
		holder, _ := nstartAcquire(context.Background(), key, 1, 0)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			release, err := nstartAcquire(ctx, key, 1, 0)
			if err == nil {
				release()
			} else if !errors.Is(err, context.Canceled) {
				t.Error(err)
			}
		}()
		go holder()
		cancel()
		<-done
		holder() // Wait for the racing release and verify it is idempotent.
		probe, stop := context.WithTimeout(context.Background(), time.Second)
		release, err := nstartAcquire(probe, key, 1, 0)
		stop()
		if err != nil {
			t.Fatal("admission race leaked a slot", err)
		}
		release()
	}
}

func TestSendContextStopsRetransmissions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var writes int
	s, key := contextTestServer(t, func(*Server, []byte) error {
		writes++
		if writes == 2 {
			cancel()
		}
		return nil
	})
	opts := s.NewOptions().WithRetry(3, time.Millisecond, 1)
	_, err := s.SendToPeerContext(ctx, key, "127.0.0.1:1", testRequest(), opts)
	assertSendError(t, err, SendPhaseExchange, context.Canceled)
	if writes != 2 {
		t.Fatalf("unexpected retry count: %d", writes)
	}
	assertPendingEmpty(t, s)
}

func TestNstartReleaseIsOnceAndSurvivesClear(t *testing.T) {
	key := t.Name()
	defer NstartClear(key)
	oldRelease, _ := nstartAcquire(context.Background(), key, 1, 0)
	NstartClear(key)
	newRelease, _ := nstartAcquire(context.Background(), key, 1, 0)
	defer newRelease()
	oldRelease()
	oldRelease()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := nstartAcquire(ctx, key, 1, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("released replacement entry")
	}
}

func TestLegacySendErrorsAndSuccessfulSends(t *testing.T) {
	s, key := contextTestServer(t, func(s *Server, raw []byte) error { ackPacket(t, s, raw); return nil })
	if _, err := s.SendToPeer(key, "127.0.0.1:1", testRequest(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SendToPeerContext(context.Background(), key, "127.0.0.1:1", NewMessage().WithType(TypeNonConfirmable), nil); err != nil {
		t.Fatal(err)
	}
	s.config.ProxyCallbacks["test"] = func([]byte, string) error { return nil }
	opts := s.NewOptions().WithRetry(0, time.Millisecond, 1)
	if _, err := s.SendToPeer(key, "127.0.0.1:1", testRequest(), opts); err != ErrTimeout {
		t.Fatalf("legacy error changed: %v", err)
	}
	assertPendingEmpty(t, s)
	failure := errors.New("transport failure")
	s.config.ProxyCallbacks["test"] = func([]byte, string) error { return failure }
	if _, err := s.SendToPeer(key, "127.0.0.1:1", testRequest(), nil); err != failure {
		t.Fatal(err)
	}
	assertPendingEmpty(t, s)
}
