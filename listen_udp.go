// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"errors"
	"net"
	"time"
)

type UdpListener struct {
	name     string
	socket   *net.UDPConn
	handler  *Server
	shutdown bool
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

	for {
		rawLen, from, err := l.socket.ReadFromUDP(rawReq)
		if err != nil {
			if l.shutdown {
				logDebug(nil, nil, "coap: reader shutdown")
				return
			}
			logWarn(nil, err, "coap: error reading COAP packet")
			go l.reader()
			return
		}
		newReq := append([]byte(nil), rawReq[:rawLen]...)
		sniffActivity("udp", SniffRead, from.String(), l.socket.LocalAddr().String(), newReq)
		go l.handle(newReq, from)
	}
}

func (l *UdpListener) handle(rawReq []byte, from *net.UDPAddr) {

	var req Message
	if err := req.unmarshalBinary(rawReq); err != nil {
		logError(nil, err, "coap: error parsing COAP header")
		return
	}
	req.Meta.RemoteAddr = from.String()
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
			sniffActivity("udp", SniffWrite, l.socket.LocalAddr().String(), from.String(), rawRsp)
			_, err = l.socket.WriteToUDP(rawRsp, from)
			if err != nil {
				logWarn(nil, err, "coap: error writing coap response")
			}
		}
	}

	return
}

func (l *UdpListener) Send(addr string, data []byte) error {
	if l.shutdown {
		return errors.New("coap: port is shutdown")
	}
	uaddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	sniffActivity("udp", SniffWrite, l.socket.LocalAddr().String(), addr, data)
	_, err = l.socket.WriteToUDP(data, uaddr)
	if err != nil {
		return err
	}
	return nil
}

func (l *UdpListener) Close() {
	l.shutdown = true
	_ = l.socket.Close()
}
