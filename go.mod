module github.com/qwerty-iot/coap

go 1.23

//replace github.com/qwerty-iot/dtls/v2 => ../dtls

require (
	github.com/qwerty-iot/dtls/v2 v2.10.3
	github.com/qwerty-iot/tox v1.4.5
)

require (
	dario.cat/mergo v1.0.2 // indirect
	github.com/goccy/go-json v0.12.0 // indirect
)
