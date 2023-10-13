module github.com/qwerty-iot/coap

go 1.20

//replace github.com/qwerty-iot/dtls/v2 => ../dtls

require (
	github.com/qwerty-iot/dtls/v2 v2.7.4
	github.com/qwerty-iot/tox v1.2.0
)

require (
	dario.cat/mergo v1.0.0 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
)
