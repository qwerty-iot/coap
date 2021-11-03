// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"crypto/rand"
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

	pendingMap   map[string]*pendingEntry
	pendingMux   sync.Mutex
	pendingMsgId uint16

	blockCache sync.Map
}

type Config struct {
	DeduplicateExpiration   time.Duration
	DeduplicateInterval     time.Duration
	ObserveNotFoundCallback ObserveNotFoundCallback
	BlockDefaultSize        int
	BlockInactivityTimeout  time.Duration
	NStart                  int
}

func NewServer(conf *Config, udpAddr string, dtlsListener *dtls.Listener) (*Server, error) {
	h := &Server{}
	h.config = &Config{
		DeduplicateExpiration:  time.Second * 600,
		DeduplicateInterval:    time.Second * 20,
		BlockDefaultSize:       1024,
		BlockInactivityTimeout: time.Second * 120,
		NStart:                 10,
	}
	h.routes = map[string]*routeEntry{}
	h.pendingMap = map[string]*pendingEntry{}
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
	}

	go h.dedupWatcher()
	go h.expireBlocks()
	return h, nil
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
