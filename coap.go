// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"crypto/rand"
	"time"
)

type Config struct {
	DeduplicateExpiration   time.Duration
	DeduplicateInterval     time.Duration
	ObserveNotFoundCallback ObserveNotFoundCallback
	BlockDefaultSize        int
	BlockInactivityTimeout  time.Duration
}

var config = &Config{
	DeduplicateExpiration:  time.Second * 600,
	DeduplicateInterval:    time.Second * 20,
	BlockDefaultSize:       1024,
	BlockInactivityTimeout: time.Second * 120,
}

func Configure(conf *Config) {
	if conf != nil {
		if conf.DeduplicateExpiration > 0 {
			config.DeduplicateExpiration = conf.DeduplicateExpiration
		}
		if conf.DeduplicateInterval > 0 {
			config.DeduplicateInterval = conf.DeduplicateInterval
		}
		if conf.ObserveNotFoundCallback != nil {
			config.ObserveNotFoundCallback = conf.ObserveNotFoundCallback
		}
		if conf.BlockDefaultSize > 0 {
			config.BlockDefaultSize = conf.BlockDefaultSize
		}
		if conf.BlockInactivityTimeout > 0 {
			config.BlockInactivityTimeout = conf.BlockInactivityTimeout
		}
	}

	go dedupWatcher()
	go expireBlocks()
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
