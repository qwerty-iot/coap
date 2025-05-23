module github.com/qwerty-iot/coap

go 1.20

//replace github.com/qwerty-iot/dtls/v2 => ../dtls

require (
	github.com/qwerty-iot/dtls/v2 v2.7.8
	github.com/qwerty-iot/tox v1.2.21
)

require (
	dario.cat/mergo v1.0.2 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
)
