package coap

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestBlock2Reassembly(t *testing.T) {
	for _, notify := range []bool{false, true} {
		kind := "response"
		if notify {
			kind = "notification"
		}
		for _, outcome := range []string{"short-final", "empty-final", "missing-block2", "timeout", "transport-error"} {
			t.Run(kind+"/"+outcome, func(t *testing.T) {
				first := bytes.Repeat([]byte{0x11}, 16)
				middle := bytes.Repeat([]byte{0x22}, 16)
				last := []byte{0x33, 0x44, 0x55}
				if outcome == "empty-final" {
					last = nil
				}
				want := append(append(append([]byte{}, first...), middle...), last...)
				fail := outcome != "short-final" && outcome != "empty-final"
				writes := 0
				s, key := contextTestServer(t, func(s *Server, raw []byte) error {
					var req Message
					if err := req.unmarshalBinary(raw); err != nil {
						return err
					}
					if req.Type == TypeAcknowledgement {
						return nil
					}
					writes++
					num := 0
					if block := req.GetBlock2(); block != nil {
						num = block.Num
					}
					if num == 2 {
						if outcome == "timeout" {
							return ErrTimeout
						}
						if outcome == "transport-error" {
							return errors.New("test transport failed")
						}
					}
					rsp := &Message{Type: TypeAcknowledgement, Code: RspCodeContent, Token: req.Token, MessageID: req.MessageID}
					switch num {
					case 0:
						rsp.Payload = first
					case 1:
						rsp.Payload = middle
					case 2:
						rsp.Payload = last
					default:
						t.Fatalf("unexpected block request %d", num)
					}
					if outcome != "missing-block2" || num != 2 {
						rsp.WithBlock2(blockInit(num, num < 2, 16))
					}
					if !s.handleAcknowledgement(rsp) {
						t.Fatal("response did not match pending exchange")
					}
					return nil
				})
				if notify {
					calls := 0
					token := t.Name()
					ObserveRegister(token, "99999/0", func(req *Message, _ interface{}) error {
						calls++
						if fail {
							t.Error("partial notification dispatched after retrieval failure")
						}
						if !bytes.Equal(req.Payload, want) {
							t.Errorf("assembled payload length = %d, want %d", len(req.Payload), len(want))
						}
						return nil
					}, nil)
					t.Cleanup(func() { observeMap.Delete(token) })
					req := &Message{Type: TypeConfirmable, Code: RspCodeContent, Token: []byte(token), MessageID: 42, Payload: first,
						Meta: Metadata{RemoteAddr: "127.0.0.1:1", PeerKey: key}}
					req.WithOption(OptObserve, 1, true).WithBlock2(blockInit(0, true, 16))
					rsp := s.handleMessage(req)
					if fail {
						if calls != 0 || rsp == nil || rsp.Type != TypeReset {
							t.Fatalf("calls = %d, reply = %v", calls, rsp)
						}
						if !bytes.Equal(req.Payload, first) {
							t.Fatal("partial reassembly published into original message")
						}
					} else if calls != 1 {
						t.Fatalf("callback calls = %d, want 1", calls)
					}
					if writes != 2 {
						t.Fatalf("continuation requests = %d, want 2", writes)
					}
				} else {
					req := NewMessage().WithType(TypeConfirmable).WithCode(CodeGet).WithPathString("99999/0")
					rsp, err := s.SendToPeerContext(context.Background(), key, "127.0.0.1:1", req, nil)
					if fail {
						if err == nil || rsp != nil {
							t.Fatalf("got %v, %v; want nil and error", rsp, err)
						}
						var sendErr *SendError
						if !errors.As(err, &sendErr) || sendErr.Phase != SendPhaseExchange {
							t.Fatalf("lost exchange error classification: %v", err)
						}
					} else if err != nil || rsp == nil || !bytes.Equal(rsp.Payload, want) {
						t.Fatalf("unexpected response: %v, %v", rsp, err)
					}
					if writes != 3 {
						t.Fatalf("requests = %d, want 3", writes)
					}
				}
				assertPendingEmpty(t, s)
			})
		}
	}
}
