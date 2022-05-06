module github.com/qwerty-iot/coap

go 1.17

//replace github.com/qwerty-iot/dtls/v2 => ../dtls

require (
	github.com/qwerty-iot/dtls/v2 v2.6.0
	github.com/qwerty-iot/tox v1.0.13
)

require (
	github.com/fxamacker/cbor/v2 v2.4.0 // indirect
	github.com/qwerty-iot/lwm2m/v2 v2.6.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.21.0 // indirect
)
