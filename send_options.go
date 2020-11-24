// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import "time"

type SendOptions struct {
	retryCount   int
	retryTimeout time.Duration
}

func NewOptions() *SendOptions {
	return &SendOptions{
		retryCount:   3,
		retryTimeout: time.Second * 5,
	}
}

func (so *SendOptions) WithRetry(count int, timeout time.Duration) *SendOptions {
	so.retryCount = count
	so.retryTimeout = timeout
	return so
}

func (so *SendOptions) NoRetry() *SendOptions {
	so.retryCount = -1
	return so
}
