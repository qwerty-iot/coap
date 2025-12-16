// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import "time"

type SendOptions struct {
	MaxRetransmit  int           `json:"MaxRetransmit"`
	ActTimeout     time.Duration `json:"ActTimeout"`
	RandomFactor   float64       `json:"RandomFactor"`
	BlockSize      int           `json:"BlockSize"`
	MaxMessageSize int           `json:"MaxMessageSize"`
	NStart         int           `json:"NStart"`
}

func (s *Server) NewOptions() *SendOptions {
	return &SendOptions{
		MaxRetransmit:  3,
		ActTimeout:     time.Second * 5,
		RandomFactor:   1.5,
		BlockSize:      s.config.BlockDefaultSize,
		MaxMessageSize: s.config.MaxMessageDefaultSize,
		NStart:         s.config.NStart,
	}
}

func (so *SendOptions) WithRetry(count int, timeout time.Duration, randomFactor float64) *SendOptions {
	so.MaxRetransmit = count
	so.ActTimeout = timeout
	so.RandomFactor = randomFactor
	return so
}

func (so *SendOptions) WithBlockSize(bs int) *SendOptions {
	so.BlockSize = bs
	return so
}

func (so *SendOptions) WithMaxMessageSize(ms int) *SendOptions {
	so.MaxMessageSize = ms
	return so
}

func (so *SendOptions) WithNStart(ns int) *SendOptions {
	so.NStart = ns
	return so
}

func (so *SendOptions) NoRetry() *SendOptions {
	so.MaxRetransmit = -1
	return so
}
