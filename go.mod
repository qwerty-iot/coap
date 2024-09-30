module github.com/qwerty-iot/coap

go 1.20

//replace github.com/qwerty-iot/dtls/v2 => ../dtls

require (
	github.com/qwerty-iot/dtls/v2 v2.7.7
	github.com/qwerty-iot/tox v1.2.16
)

require (
	dario.cat/mergo v1.0.1 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
)
