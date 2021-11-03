// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import "time"

type SendOptions struct {
	maxRetransmit int
	ackTimeout    time.Duration
	randomFactor  float64
	blockSize     int
	nStart        int
}

func (s *Server) NewOptions() *SendOptions {
	return &SendOptions{
		maxRetransmit: 3,
		ackTimeout:    time.Second * 5,
		randomFactor:  1.5,
		blockSize:     s.config.BlockDefaultSize,
		nStart:        s.config.NStart,
	}
}

func (so *SendOptions) WithRetry(count int, timeout time.Duration, randomFactor float64) *SendOptions {
	so.maxRetransmit = count
	so.ackTimeout = timeout
	so.randomFactor = randomFactor
	return so
}

func (so *SendOptions) WithBlockSize(bs int) *SendOptions {
	so.blockSize = bs
	return so
}

func (so *SendOptions) WithNStart(ns int) *SendOptions {
	so.nStart = ns
	return so
}

func (so *SendOptions) NoRetry() *SendOptions {
	so.maxRetransmit = -1
	return so
}
