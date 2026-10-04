package coap

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitForNstartWaiters(t *testing.T, key string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if nstartCount(key) == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("waiters for %s = %d, want %d", key, nstartCount(key), want)
}

func queueNstartWaiter(t *testing.T, key string, limit int) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done, finished := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(finished)
		release, err := nstartAcquire(ctx, key, 1, limit)
		if release != nil {
			release()
		}
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("NSTART waiter did not exit")
		}
	})
	return cancel, done
}

func TestNstartQueueLimitCancellationAndProgress(t *testing.T) {
	key := t.Name()
	t.Cleanup(func() { NstartClear(key) })
	holder, err := nstartAcquire(context.Background(), key, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	defer holder()
	var cancelFirst context.CancelFunc
	var done []<-chan error
	for i := 0; i < 20; i++ {
		cancel, result := queueNstartWaiter(t, key, 20)
		if i == 0 {
			cancelFirst = cancel
		}
		done = append(done, result)
	}
	waitForNstartWaiters(t, key, 20)
	if got := NstartQueueStats(); got != (NstartStats{Waiters: 20, QueuedPeers: 1, MaxPeerWaiters: 20}) {
		t.Fatalf("full queue stats: %+v", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if release, err := nstartAcquire(ctx, key, 1, 20); release != nil || !errors.Is(err, ErrNStartQueueFull) {
		t.Fatalf("overflow: %v", err)
	}
	// Cancellation must take precedence over overload, without changing the queue.
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := nstartAcquire(canceled, key, 1, 20); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled overflow: %v", err)
	}
	cancelFirst()
	if err := <-done[0]; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter: %v", err)
	}
	waitForNstartWaiters(t, key, 19)
	_, replacement := queueNstartWaiter(t, key, 20)
	waitForNstartWaiters(t, key, 20)
	holder()
	for _, result := range append(done[1:], replacement) {
		if err := <-result; err != nil {
			t.Fatalf("waiter failed to progress: %v", err)
		}
	}
	if got := NstartQueueStats(); got != (NstartStats{}) {
		t.Fatalf("drained queue stats: %+v", got)
	}
}

func TestNstartQueuesAreIndependentAndZeroLimitIsUnlimited(t *testing.T) {
	for i, limit := range []int{0, 2} {
		key := t.Name() + string(rune('a'+i))
		t.Cleanup(func() { NstartClear(key) })
		holder, err := nstartAcquire(context.Background(), key, 1, limit)
		if err != nil {
			t.Fatal(err)
		}
		defer holder()
		want := 25
		if limit > 0 {
			want = limit
		}
		for j := 0; j < want; j++ {
			queueNstartWaiter(t, key, limit)
		}
		waitForNstartWaiters(t, key, want)
	}
	if got := NstartQueueStats(); got != (NstartStats{Waiters: 27, QueuedPeers: 2, MaxPeerWaiters: 25}) {
		t.Fatalf("independent queue stats: %+v", got)
	}
}

func TestNstartQueueDeadlineRemovesWaiter(t *testing.T) {
	key := t.Name()
	t.Cleanup(func() { NstartClear(key) })
	holder, _ := nstartAcquire(context.Background(), key, 1, 1)
	defer holder()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := nstartAcquire(ctx, key, 1, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued deadline: %v", err)
	}
	if got := nstartCount(key); got != 0 {
		t.Fatalf("expired waiter retained: %d", got)
	}
	_, done := queueNstartWaiter(t, key, 1)
	waitForNstartWaiters(t, key, 1)
	holder()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSendQueueFullDoesNotTransmitOrRegisterPending(t *testing.T) {
	var writes atomic.Int32
	s, key := contextTestServer(t, func(*Server, []byte) error { writes.Add(1); return nil })
	s.config.NStartMaxWaiters = 1
	holder, _ := nstartAcquire(context.Background(), key, 1, 1)
	defer holder()
	queueNstartWaiter(t, key, 1)
	waitForNstartWaiters(t, key, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := s.SendToPeerContext(ctx, key, "127.0.0.1:1", testRequest(), nil)
	assertSendError(t, err, SendPhaseNStart, ErrNStartQueueFull)
	if errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queue full classified as timeout: %v", err)
	}
	if writes.Load() != 0 {
		t.Fatal("rejected exchange transmitted")
	}
	assertPendingEmpty(t, s)
	_, err = s.SendToPeer(key, "127.0.0.1:1", testRequest(), nil)
	if err != ErrNStartQueueFull {
		t.Fatalf("legacy entrypoint lost sentinel: %v", err)
	}
	// Non-confirmable messages do not acquire NSTART.
	_, err = s.SendToPeerContext(ctx, key, "127.0.0.1:1", testRequest().WithType(TypeNonConfirmable), nil)
	if err != nil || writes.Load() != 1 {
		t.Fatalf("non-confirmable send affected: %v, writes %d", err, writes.Load())
	}
}

func TestNstartQueueAdmissionCancellationAndStatsRace(t *testing.T) {
	key := t.Name()
	t.Cleanup(func() { NstartClear(key) })
	for round := 0; round < 30; round++ {
		holder, _ := nstartAcquire(context.Background(), key, 1, 20)
		ctx, cancel := context.WithCancel(context.Background())
		var workers sync.WaitGroup
		var active atomic.Int32
		for i := 0; i < 50; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				release, err := nstartAcquire(ctx, key, 1, 20)
				if err != nil {
					if !errors.Is(err, ErrNStartQueueFull) && !errors.Is(err, context.Canceled) {
						t.Errorf("unexpected acquisition error: %v", err)
					}
					return
				}
				if active.Add(1) != 1 {
					t.Error("more than one active exchange")
				}
				active.Add(-1)
				release()
				release()
			}()
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := 0; i < 50; i++ {
				stats := NstartQueueStats()
				if stats.Waiters < 0 || stats.Waiters > 20 || stats.MaxPeerWaiters != stats.Waiters || stats.QueuedPeers < 0 || stats.QueuedPeers > 1 {
					t.Errorf("invalid concurrent snapshot: %+v", stats)
				}
			}
		}()
		go cancel()
		holder()
		workers.Wait()
		if got := NstartQueueStats(); got != (NstartStats{}) {
			t.Fatalf("retained waiters after race: %+v", got)
		}
	}
}

func TestServerCopiesNstartWaiterLimit(t *testing.T) {
	for _, limit := range []int{0, 20, 3} {
		s, err := NewServer(&Config{NStartMaxWaiters: limit}, "127.0.0.1:0", nil)
		if err != nil {
			t.Fatal(err)
		}
		if s.GetConfig().NStartMaxWaiters != limit || s.GetConfig().NStart != 1 {
			t.Fatalf("server NSTART config: %+v", s.GetConfig())
		}
		s.Close()
	}
}
