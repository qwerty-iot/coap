package coap

import "testing"

func TestProxyPeerKeyKeepsLiteralRemoteAddress(t *testing.T) {
	var callbackPeer string
	config := NewConfig()
	config.AddProxyReceiver("gateway", func(_ []byte, to string) error {
		callbackPeer = to
		return nil
	})
	server, err := NewServer(config, "127.0.0.1:0", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	var received Metadata
	server.AddRoute("/", func(req *Message) *Message {
		received = req.Meta
		return req.MakeReply(RspCodeChanged, nil)
	})

	req := NewMessage().WithType(TypeConfirmable).WithCode(CodePost)
	req.MessageID = 1
	raw, err := req.marshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = server.ProxySendWithPeer("gateway", raw, "10.0.0.5:49152", "site-a:gateway-a:listener-a:flow-a"); err != nil {
		t.Fatal(err)
	}

	if received.RemoteAddr != "10.0.0.5:49152" {
		t.Fatalf("unexpected remote address %q", received.RemoteAddr)
	}
	if received.GetPeerKey() != "gateway:site-a:gateway-a:listener-a:flow-a" {
		t.Fatalf("unexpected peer key %q", received.GetPeerKey())
	}

	msg := NewMessage().WithType(TypeNonConfirmable).WithCode(CodePost)
	if _, err = server.SendToPeer(received.GetPeerKey(), received.RemoteAddr, msg, server.NewOptions()); err != nil {
		t.Fatal(err)
	}
	if callbackPeer != "site-a:gateway-a:listener-a:flow-a" {
		t.Fatalf("proxy callback received %q", callbackPeer)
	}
}

func TestPeerKeySeparatesDeduplicationForIdenticalAddresses(t *testing.T) {
	server, err := NewServer(nil, "127.0.0.1:0", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	now := server.LastActivity()
	first := &Message{MessageID: 7, Meta: Metadata{RemoteAddr: "10.0.0.5:49152", PeerKey: "gateway:site-a", ReceivedAt: now}}
	second := &Message{MessageID: 7, Meta: Metadata{RemoteAddr: "10.0.0.5:49152", PeerKey: "gateway:site-b", ReceivedAt: now}}
	if _, ok := server.deduplicate(first); !ok {
		t.Fatal("first peer was unexpectedly treated as a duplicate")
	}
	if _, ok := server.deduplicate(second); !ok {
		t.Fatal("second scoped peer collided with the first")
	}
}
