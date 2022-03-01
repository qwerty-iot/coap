// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"time"

	"github.com/qwerty-iot/dtls/v2"
)

type DtlsListener struct {
	name     string
	socket   *dtls.Listener
	handler  *Server
	shutdown bool
}

func (l *DtlsListener) listen(name string, listener *dtls.Listener, handler *Server) error {
	l.socket = listener
	l.name = name
	l.handler = handler
	go l.reader()
	return nil
}

func (l *DtlsListener) reader() {

	rawReq, peer := l.socket.Read()
	if l.shutdown {
		logDebug(nil, nil, "coap: port is shutdown")
		return
	}

	//launch new reader
	go l.reader()

	var req Message
	if err := req.unmarshalBinary(rawReq); err != nil {
		logError(nil, err, "coap: error parsing COAP header")
		return
	}
	req.Meta.RemoteAddr = peer.RemoteAddr()
	req.Meta.DtlsIdentity = peer.SessionIdentityString()
	req.Meta.DtlsPublicKey = peer.SessionPublicKey()
	req.Meta.DtlsCertificate = peer.SessionCertificate()
	req.Meta.ListenerName = l.name
	req.Meta.ReceivedAt = time.Now().UTC()
	req.Meta.Server = l.handler

	rsp := l.handler.handleMessage(&req)

	if rsp != nil {
		rawRsp, err := rsp.marshalBinary()
		if err != nil {
			logError(nil, err, "coap: error marshaling COAP response")
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

func (l *DtlsListener) FindPeer(addr string) *dtls.Peer {
	if l == nil {
		return nil
	}
	peer, _ := l.socket.FindPeer(addr)
	return peer
}

func (l *DtlsListener) Close() {
	l.shutdown = true
	_ = l.socket.Shutdown()

}
