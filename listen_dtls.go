package coap

import (
	"github.com/qwerty-iot/dtls/v2"
	"time"
)

var dtlsListener *dtls.Listener

func ListenDtls(name string, listener *dtls.Listener) error {
	go dtlsReader(name, listener)
	return nil
}

func dtlsReader(name string, listener *dtls.Listener) {

	dtlsListener = listener

	rawReq, peer := listener.Read()

	//launch new reader
	go dtlsReader(name, listener)

	var req Message
	if err := req.unmarshalBinary(rawReq); err != nil {
		logError(nil, err, "error parsing COAP header")
		return
	}
	req.Meta.RemoteAddr = peer.RemoteAddr()
	req.Meta.DtlsIdentity = peer.SessionIdentityString()
	req.Meta.ListenerName = name
	req.Meta.ReceivedAt = time.Now().UTC()

	rsp := handleMessage(&req)

	if rsp != nil {
		rawRsp, err := rsp.marshalBinary()
		if err != nil {
			logError(nil, err, "error marshaling COAP response")
			return
		}

		if rawRsp != nil {
			err := peer.Write(rawRsp)
			if err != nil {
				logWarn(nil, err, "coap: error writing coap response")
			}
		}
	}

	return
}

func dtlsFindPeer(addr string) *dtls.Peer {
	peer, _ := dtlsListener.FindPeer(addr)
	return peer
}
