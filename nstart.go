package coap

import (
	"context"
	"sync"
)

type nstart struct {
	count   int
	waiters int
	changed chan struct{}
	mux     sync.Mutex
}

var nstartMap = map[string]*nstart{}
var nstartMux sync.Mutex

// nstartAcquire returns a release function tied to this entry, even if the
// address is subsequently cleared and assigned a new entry.
func nstartAcquire(ctx context.Context, addr string, nStart, maxWaiters int) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if nStart <= 0 {
		return func() {}, nil
	}
	nstartMux.Lock()
	s := nstartMap[addr]
	if s == nil {
		s = &nstart{changed: make(chan struct{})}
		nstartMap[addr] = s
	}
	nstartMux.Unlock()
	s.mux.Lock()
	waiting := false
	defer func() {
		if waiting {
			s.waiters--
		}
		s.mux.Unlock()
	}()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if s.count < nStart {
			s.count++
			var once sync.Once
			return func() {
				once.Do(func() {
					s.mux.Lock()
					s.count--
					close(s.changed)
					s.changed = make(chan struct{})
					s.mux.Unlock()
				})
			}, nil
		}
		if !waiting {
			if maxWaiters > 0 && s.waiters >= maxWaiters {
				return nil, ErrNStartQueueFull
			}
			s.waiters++
			waiting = true
		}
		changed := s.changed
		s.mux.Unlock()
		select {
		case <-ctx.Done():
		case <-changed:
		}
		s.mux.Lock()
	}
}

func nstartCount(addr string) int {
	nstartMux.Lock()
	if s, found := nstartMap[addr]; found {
		nstartMux.Unlock()
		s.mux.Lock()
		defer s.mux.Unlock()
		return s.waiters
	}
	nstartMux.Unlock()
	return -1
}

// NstartStats describes current waiting exchanges across tracked peer entries.
type NstartStats struct {
	Waiters        int
	QueuedPeers    int
	MaxPeerWaiters int
}

// NstartQueueStats samples each current peer once. Idle entries do not contribute.
func NstartQueueStats() NstartStats {
	nstartMux.Lock()
	entries := make([]*nstart, 0, len(nstartMap))
	for _, s := range nstartMap {
		entries = append(entries, s)
	}
	nstartMux.Unlock()
	var stats NstartStats
	for _, s := range entries {
		s.mux.Lock()
		waiters := s.waiters
		s.mux.Unlock()
		stats.Waiters += waiters
		if waiters > 0 {
			stats.QueuedPeers++
		}
		if waiters > stats.MaxPeerWaiters {
			stats.MaxPeerWaiters = waiters
		}
	}
	return stats
}

func NstartClear(addr string) {
	nstartMux.Lock()
	delete(nstartMap, addr)
	nstartMux.Unlock()
}

func NstartEntryCount() int {
	nstartMux.Lock()
	l := len(nstartMap)
	nstartMux.Unlock()
	return l
}
