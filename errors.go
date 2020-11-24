package coap

import "errors"

var (
	ErrTimeout               = errors.New("coap: timeout")
	ErrBadRequest            = errors.New("coap: bad request")
	ErrNotFound              = errors.New("coap: not found")
	ErrUnauthorized          = errors.New("coap: not authorized")
	ErrMethodNotAllowed      = errors.New("coap: not authorized")
	ErrEncodingNotAcceptable = errors.New("coap: not authorized")
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
	default:
		return errors.New("coap: other error " + code.String())
	}
}
