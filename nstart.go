package coap

import "sync"

type nstart struct {
	count int
	cond  *sync.Cond
	mux   sync.Mutex
}

var nstartMap = map[string]*nstart{}
var nstartMux sync.Mutex

func nstartInc(addr string, nStart int) {
	if nStart > 0 {
		nstartMux.Lock()
		if s, found := nstartMap[addr]; found {
			nstartMux.Unlock()
			s.cond.L.Lock()
			for s.count >= nStart {
				s.cond.Wait()
			}
			s.count++
			s.cond.L.Unlock()
		} else {
			ns := &nstart{count: 1}
			ns.cond = sync.NewCond(&ns.mux)
			nstartMap[addr] = ns
			nstartMux.Unlock()
		}
	}
}

func nstartCount(addr string, nStart int) int {
	nstartMux.Lock()
	if s, found := nstartMap[addr]; found {
		nstartMux.Unlock()
		if s.count > nStart {
			return s.count - nStart
		} else {
			return 0
		}
	}
	nstartMux.Unlock()
	return -1
}

func nstartDec(addr string) {
	nstartMux.Lock()
	if s, found := nstartMap[addr]; found {
		nstartMux.Unlock()
		s.cond.L.Lock()
		s.count--
		s.cond.Signal()
		s.cond.L.Unlock()
	} else {
		nstartMux.Unlock()
	}
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
