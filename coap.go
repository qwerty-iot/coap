// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"crypto/rand"
	"time"
)

type Config struct {
	ExchangeLifetime        int
	DeduplicateExpiration   time.Duration
	DeduplicateInterval     time.Duration
	ObserveNotFoundCallback ObserveNotFoundCallback
}

var config = &Config{
	ExchangeLifetime:      10,
	DeduplicateExpiration: time.Second * 600,
	DeduplicateInterval:   time.Second * 20,
}

func Configure(conf *Config) {
	config = conf

	go dedupWatcher()
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
