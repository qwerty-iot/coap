// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"errors"
	"time"
)

var ProxyRecv func(rawReq []byte, to string) error

func (s *Server) ProxySend(rawReq []byte, from string) ([]byte, error) {

	var req Message
	if err := req.unmarshalBinary(rawReq); err != nil {
		logError(nil, err, "coap: error parsing COAP header")
		return nil, err
	}
	req.Meta.RemoteAddr = "proxy:" + from
	req.Meta.ListenerName = "proxy"
	req.Meta.ReceivedAt = time.Now().UTC()

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

func proxyRecv(addr string, data []byte) error {
	if ProxyRecv == nil {
		return errors.New("coap: no proxy receive callback registered")
	}
	return ProxyRecv(data, addr[6:])
}
