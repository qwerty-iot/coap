package coap

import "github.com/qwerty-iot/dtls/v2"

const (
	SniffWrite = "write"
	SniffRead  = "read"
)

type SniffPacketsCallback func(transportType string, op string, from string, to string, data []byte)

var sniffActivityCallback SniffPacketsCallback

func SetSniffPacketsCallback(callback SniffPacketsCallback) {
	dtls.SetSniffPacketsCallback(func(transportType string, op string, from string, to string, data []byte) {
		callback("dtls", op, from, to, data)
	})
	sniffActivityCallback = callback
}

func sniffActivity(transportType string, op string, from string, to string, data []byte) {
	if sniffActivityCallback != nil {
		go sniffActivityCallback(transportType, op, from, to, data)
	}
}
