module github.com/qwerty-iot/coap

go 1.15

replace github.com/qwerty-iot/dtls/v2 => ../dtls

require (
	github.com/bocajim/dtls v0.0.0-20190919154819-4ef9c2aba394
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/plgd-dev/kit v0.0.0-20201102152602-1e03187a6a3a
	github.com/qwerty-iot/dtls/v2 v2.0.0
)
