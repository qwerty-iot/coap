// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"sync"
)

var observeMap sync.Map

type ObserveCallback func(req *Message, arg interface{}) error
type ObserveNotFoundCallback func(req *Message) bool

type Observation struct {
	callback ObserveCallback
	arg      interface{}
}

func Observe(addr string, path string, encoding MediaType, callback ObserveCallback, arg interface{}, options *SendOptions) (string, error) {
	if options == nil {
		options = NewOptions()
	}

	req := &Message{Type: TypeConfirmable, Code: CodeGet}
	req.WithOption(OptObserve, 0, true)
	req.WithPathString(path)
	if encoding != None {
		req.WithOption(OptAccept, encoding, true)
	}

	rsp, err := Send(addr, req, options)
	if err != nil {
		return "", err
	}

	err = RspCodeToError(rsp.Code)
	if err != nil {
		return "", err
	}

	observeMap.Store(string(req.Token), &Observation{callback: callback, arg: arg})

	_ = callback(rsp, arg)

	return string(req.Token), nil
}

func ObserveCancel(addr string, path string, token string, options *SendOptions) error {
	if options == nil {
		options = NewOptions()
	}

	req := &Message{Type: TypeConfirmable, Code: CodeGet}
	req.WithOption(OptObserve, 1, true)
	req.WithPathString(path)
	req.Token = []byte(token)

	observeMap.Delete(token)

	rsp, err := Send(addr, req, options)
	if err != nil {
		return err
	}

	err = RspCodeToError(rsp.Code)
	if err != nil {
		return err
	}

	return nil
}

func ObserveRegister(token string, callback ObserveCallback, arg interface{}) {
	observeMap.Store(token, &Observation{callback: callback, arg: arg})
	return
}

func ObserveTokens(callback func(string)) {
	observeMap.Range(func(key interface{}, value interface{}) bool {
		callback(key.(string))
		return true
	})
}

func getObserve(msg *Message) *Observation {
	c, found := observeMap.Load(string(msg.Token))
	if found {
		return c.(*Observation)
	} else {
		if config.ObserveNotFoundCallback != nil && config.ObserveNotFoundCallback(msg) {
			c, found = observeMap.Load(string(msg.Token))
			if found {
				return c.(*Observation)
			}
		}
	}
	return nil
}
