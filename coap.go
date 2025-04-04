// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"crypto/rand"
	"github.com/qwerty-iot/tox"
	"net"
	"sync"
	"time"

	"github.com/qwerty-iot/dtls/v2"
)

type Server struct {
	config       *Config
	udpListener  *UdpListener
	dtlsListener *DtlsListener

	dedupMap         sync.Map
	dedupDeleteAfter sync.Map

	routes map[string]*routeEntry

	pendingMap    map[string]*pendingEntry
	pendingMidMap map[uint16]*pendingEntry
	pendingMux    sync.Mutex
	pendingMsgId  uint16

	blockCache sync.Map

	lastActivity time.Time
}

type Config struct {
	DeduplicateExpiration   time.Duration
	DeduplicateInterval     time.Duration
	ObserveNotFoundCallback ObserveNotFoundCallback
	BlockDefaultSize        int
	BlockInactivityTimeout  time.Duration
	NStart                  int
	Name                    string
	Ref                     any
	ProxyCallbacks          map[string]ProxyFunction
}

func NewConfig() *Config {
	return &Config{
		DeduplicateExpiration:  time.Second * 600,
		DeduplicateInterval:    time.Second * 20,
		BlockDefaultSize:       1024,
		BlockInactivityTimeout: time.Second * 120,
		NStart:                 1,
	}
}

func (c *Config) AddProxyReceiver(prefix string, f ProxyFunction) {
	if c.ProxyCallbacks == nil {
		c.ProxyCallbacks = map[string]ProxyFunction{}
	}
	c.ProxyCallbacks[prefix] = f
}

func NewServer(conf *Config, udpAddr string, dtlsListener *dtls.Listener) (*Server, error) {
	h := &Server{}
	h.config = NewConfig()
	h.routes = map[string]*routeEntry{}
	h.pendingMap = map[string]*pendingEntry{}
	h.pendingMidMap = map[uint16]*pendingEntry{}
	h.pendingMsgId = uint16(time.Now().UnixNano() % 32767)

	if len(udpAddr) != 0 {
		h.udpListener = &UdpListener{}
		if err := h.udpListener.listen("udp", udpAddr, h); err != nil {
			return nil, err
		}
	}

	if dtlsListener != nil {
		h.dtlsListener = &DtlsListener{}
		if err := h.dtlsListener.listen("dtls", dtlsListener, h); err != nil {
			return nil, err
		}
	}

	if conf != nil {
		if conf.DeduplicateExpiration > 0 {
			h.config.DeduplicateExpiration = conf.DeduplicateExpiration
		}
		if conf.DeduplicateInterval > 0 {
			h.config.DeduplicateInterval = conf.DeduplicateInterval
		}
		if conf.ObserveNotFoundCallback != nil {
			h.config.ObserveNotFoundCallback = conf.ObserveNotFoundCallback
		}
		if conf.BlockDefaultSize > 0 {
			h.config.BlockDefaultSize = conf.BlockDefaultSize
		}
		if conf.BlockInactivityTimeout > 0 {
			h.config.BlockInactivityTimeout = conf.BlockInactivityTimeout
		}
		if conf.NStart > 0 {
			h.config.NStart = conf.NStart
		}
		h.config.Ref = conf.Ref
		h.config.Name = conf.Name

		h.config.ProxyCallbacks = conf.ProxyCallbacks
	}

	go h.dedupWatcher()
	go h.expireBlocks()
	return h, nil
}

func (s *Server) GetRef() any {
	if s.config != nil {
		return s.config.Ref
	} else {
		return nil
	}
}

func (s *Server) GetConfig() *Config {
	return s.config
}

func (s *Server) GetPorts() (int, int) {
	up := -1
	if s.udpListener != nil {
		_, ups, _ := net.SplitHostPort(s.udpListener.socket.LocalAddr().String())
		up = tox.ToInt(ups)
	}
	dp := -1
	if s.dtlsListener != nil {
		_, dps, _ := net.SplitHostPort(s.dtlsListener.socket.LocalAddr())
		dp = tox.ToInt(dps)
	}
	return up, dp
}

func (s *Server) Close() {
	if s.udpListener != nil {
		s.udpListener.Close()
	}
	if s.dtlsListener != nil {
		s.dtlsListener.Close()
	}
}

func (s *Server) LastActivity() time.Time {
	return s.lastActivity
}

func randomString(length int) string {
	const a = "01234567890ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	var bytes = make([]byte, length)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = a[b%byte(len(a))]
	}
	return string(bytes)
}
