// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"errors"
	"time"
)

type ProxyFunction func(rawReq []byte, to string) error

func (s *Server) ProxySend(prefix string, rawReq []byte, from string) ([]byte, error) {

	var req Message
	if err := req.unmarshalBinary(rawReq); err != nil {
		logError(nil, err, "coap: error parsing COAP header")
		return nil, err
	}
	req.Meta.RemoteAddr = prefix + ":" + from
	req.Meta.ListenerName = prefix
	req.Meta.ReceivedAt = time.Now().UTC()
	req.Meta.Server = s
	sniffActivity("udp", SniffRead, req.Meta.RemoteAddr, s.udpListener.socket.LocalAddr().String(), rawReq)

	rsp := s.handleMessage(&req)

	if rsp != nil {
		rawRsp, err := rsp.marshalBinary()
		if err != nil {
			logError(nil, err, "coap: error marshaling COAP response")
			return nil, err
		}

		if rawRsp != nil {
			return rawRsp, nil
		}
	}

	return nil, nil
}

func proxyRecv(s *Server, prefix string, addr string, data []byte) error {
	sniffActivity("udp", SniffWrite, s.udpListener.socket.LocalAddr().String(), addr, data)
	cb, found := s.config.ProxyCallbacks[prefix]
	if !found {
		return errors.New("callback not found for prefix: " + prefix)
	}
	return cb(data, addr[len(prefix)+1:])
}
