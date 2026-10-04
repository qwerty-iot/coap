// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"context"
	"errors"
)

type SendPhase string

const (
	SendPhaseNStart   SendPhase = "nstart"
	SendPhaseExchange SendPhase = "exchange"
)

// SendError identifies where a context-aware send failed.
type SendError struct {
	Phase SendPhase
	Err   error
}

func (e *SendError) Error() string { return "coap: " + string(e.Phase) + ": " + e.Err.Error() }
func (e *SendError) Unwrap() error { return e.Err }
func (e *SendError) Is(target error) bool {
	return target == ErrTimeout && errors.Is(e.Err, context.DeadlineExceeded)
}

// Legacy send entrypoints retain their original error values.
func legacySendError(err error) error {
	if sendErr, ok := err.(*SendError); ok {
		return sendErr.Err
	}
	return err
}

var (
	ErrTimeout               = errors.New("coap: timeout")
	ErrNStartQueueFull       = errors.New("coap: nstart queue full")
	ErrBadRequest            = errors.New("coap: bad request")
	ErrNotFound              = errors.New("coap: not found")
	ErrUnauthorized          = errors.New("coap: not authorized")
	ErrMethodNotAllowed      = errors.New("coap: method not allowed")
	ErrEncodingNotAcceptable = errors.New("coap: encoding not acceptable")
	ErrInternalServerError   = errors.New("coap: internal server error")
	ErrInvalidTokenLen       = errors.New("coap: invalid token length")
	ErrOptionTooLong         = errors.New("coap: option is too long")
	ErrOptionGapTooLarge     = errors.New("coap: option gap too large")
)

func RspCodeToError(code COAPCode) error {
	if code < 100 {
		return nil
	}
	switch code {
	case RspCodeBadRequest:
		return ErrBadRequest
	case RspCodeNotFound:
		return ErrNotFound
	case RspCodeUnauthorized:
		return ErrUnauthorized
	case RspCodeMethodNotAllowed:
		return ErrMethodNotAllowed
	case RspCodeNotAcceptable:
		return ErrEncodingNotAcceptable
	case RspCodeInternalServerError:
		return ErrInternalServerError
	default:
		return errors.New("coap: other error " + code.String())
	}
}
