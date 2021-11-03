// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"net"
	"time"
)

type UdpListener struct {
	name    string
	socket  *net.UDPConn
	handler *Server
}

func (l *UdpListener) listen(name string, addr string, handler *Server) error {

	uaddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	listener, err := net.ListenUDP("udp", uaddr)
	if err != nil {
		return err
	}

	l.socket = listener
	l.name = name
	l.handler = handler
	go l.reader()
	return nil
}

func (l *UdpListener) reader() {

	var rawReq = make([]byte, 8192)

	rawLen, from, err := l.socket.ReadFromUDP(rawReq)
	if err != nil {
		logWarn(nil, err, "coap: error reading COAP packet")
		go l.reader()
		return
	}
	rawReq = rawReq[:rawLen]

	go l.reader()

	var req Message
	if err := req.unmarshalBinary(rawReq); err != nil {
		logError(nil, err, "coap: error parsing COAP header")
		return
	}
	req.Meta.RemoteAddr = from.String()
	req.Meta.ListenerName = l.name
	req.Meta.ReceivedAt = time.Now().UTC()

	rsp := l.handler.handleMessage(&req)

	if rsp != nil {
		rawRsp, err := rsp.marshalBinary()
		if err != nil {
			logError(nil, err, "coap: error marshaling COAP response")
			return
		}

		if rawRsp != nil {
			_, err = l.socket.WriteToUDP(rawRsp, from)
			if err != nil {
				logWarn(nil, err, "coap: error writing coap response")
			}
		}
	}

	return
}

func (l *UdpListener) Send(addr string, data []byte) error {
	uaddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	_, err = l.socket.WriteToUDP(data, uaddr)
	if err != nil {
		return err
	}
	return nil
}
