package coap

import (
	"net"
	"time"
)

var udpListener *net.UDPConn

func ListenUdp(name string, addr string) error {

	uaddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	listener, err := net.ListenUDP("udp", uaddr)
	if err != nil {
		return err
	}

	udpListener = listener
	go udpReader(name, listener)
	return nil
}

func udpReader(name string, listener *net.UDPConn) {

	var rawReq = make([]byte, 8192)

	rawLen, from, err := listener.ReadFromUDP(rawReq)
	if err != nil {
		logWarn(nil, err, "coap: error reading COAP packet")
		go udpReader(name, listener)
		return
	}
	rawReq = rawReq[:rawLen]

	go udpReader(name, listener)

	var req Message
	if err := req.unmarshalBinary(rawReq); err != nil {
		logError(nil, err, "coap: error parsing COAP header")
		return
	}
	req.Meta.RemoteAddr = from.String()
	req.Meta.ListenerName = name
	req.Meta.ReceivedAt = time.Now().UTC()

	rsp := handleMessage(&req)

	rawRsp, err := rsp.marshalBinary()
	if err != nil {
		logError(nil, err, "coap: error marshaling COAP response")
		return
	}

	if rawRsp != nil {
		_, err = listener.WriteToUDP(rawRsp, from)
		if err != nil {
			logWarn(nil, err, "coap: error writing coap response")
		}
	}

	return
}

func udpSend(addr string, data []byte) error {
	uaddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	_, err = udpListener.WriteToUDP(data, uaddr)
	if err != nil {
		return err
	}
	return nil
}
