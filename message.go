// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"crypto/x509"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/qwerty-iot/tox"
)

type Metadata struct {
	ListenerName    string
	RemoteAddr      string
	DtlsIdentity    string
	DtlsPublicKey   []byte
	DtlsCertificate *x509.Certificate
	ReceivedAt      time.Time
	BlockSize       int
}

// Message is a CoAP message.
type Message struct {
	Type      COAPType
	Code      COAPCode
	MessageID uint16
	Token     []byte

	Payload []byte

	packetSize int

	opts options

	queryVars map[string]string
	PathVars  map[string]string

	Meta Metadata
}

func NewMessage() *Message {
	return &Message{}
}

func (m Message) getBlock1() *BlockMetadata {
	if oi := m.Option(OptBlock1); oi != nil {
		bm, _ := blockDecode(oi)
		return bm
	}
	return nil
}

func (m Message) getBlock2() *BlockMetadata {
	if oi := m.Option(OptBlock2); oi != nil {
		bm, _ := blockDecode(oi)
		return bm
	}
	return nil
}

func (m Message) getBlockKey() string {
	return m.Code.String() + m.PathString() + m.QueryString()
}

func (m Message) RequiresBlockwise() bool {
	if m.Meta.BlockSize != 0 {
		if m.Payload != nil && len(m.Payload) > m.Meta.BlockSize {
			return true
		} else {
			return false
		}
	} else {
		if m.Payload != nil && len(m.Payload) > config.BlockDefaultSize {
			return true
		} else {
			return false
		}
	}
}

// IsConfirmable returns true if this message is confirmable.
func (m Message) IsConfirmable() bool {
	return m.Type == TypeConfirmable
}

func (m Message) PacketSize() int {
	if m.packetSize != 0 {
		return m.packetSize
	} else {
		return m.headerSize() + len(m.Payload)
	}
}

// Options gets all the values for the given option.
func (m Message) Options(o OptionID) []interface{} {
	var rv []interface{}

	for _, v := range m.opts {
		if o == v.ID {
			rv = append(rv, v.Value)
		}
	}

	return rv
}

// Option gets the first value for the given option ID.
func (m Message) Option(o OptionID) interface{} {
	for _, v := range m.opts {
		if o == v.ID {
			return v.Value
		}
	}
	return nil
}

func (m Message) optionStrings(o OptionID) []string {
	var rv []string
	for _, o := range m.Options(o) {
		rv = append(rv, o.(string))
	}
	return rv
}

// AddOption adds an option.
func (m *Message) WithOption(opID OptionID, val interface{}, replace bool) *Message {
	if replace {
		m.RemoveOption(opID)
	}
	iv := reflect.ValueOf(val)
	if (iv.Kind() == reflect.Slice || iv.Kind() == reflect.Array) &&
		iv.Type().Elem().Kind() == reflect.String {
		for i := 0; i < iv.Len(); i++ {
			m.opts = append(m.opts, option{opID, iv.Index(i).Interface()})
		}
		return m
	}
	m.opts = append(m.opts, option{opID, val})
	return m
}

// RemoveOption removes all references to an option
func (m *Message) RemoveOption(opID OptionID) {
	m.opts = m.opts.Minus(opID)
}

func (m Message) ParseQuery() map[string]string {
	if m.queryVars != nil {
		return m.queryVars
	}
	m.queryVars = map[string]string{}

	qa := m.Options(OptURIQuery)

	for _, q := range qa {
		if qs, ok := q.(string); ok {
			ss := strings.Split(qs, "=")
			m.queryVars[ss[0]] = ss[1]
		}
	}
	return m.queryVars
}

func (m Message) QueryString() string {
	qi := m.Options(OptURIQuery)
	qa := tox.ToStringArray(qi)
	return strings.Join(qa, "&")
}

func (m *Message) WithQuery(q map[string]string) *Message {
	for k, v := range q {
		val := k
		if len(v) != 0 {
			val = fmt.Sprintf("%s=%s", k, v)
		}
		m.WithOption(OptURIQuery, val, false)
	}
	return m
}

// Path gets the Path set on this message if any.
func (m Message) Path() []string {
	return m.optionStrings(OptURIPath)
}

// PathString gets a path as a / separated string.
func (m Message) PathString() string {
	return strings.Join(m.Path(), "/")
}

// WithPathString sets a path by a / separated string.
func (m *Message) WithPathString(s string) *Message {
	for s[0] == '/' {
		s = s[1:]
	}
	m.WithPath(strings.Split(s, "/"))
	return m
}

// WithPath updates or adds a URIPath attribute on this message.
func (m *Message) WithPath(s []string) *Message {
	m.WithOption(OptURIPath, s, true)
	return m
}

func (m *Message) WithPayload(payload []byte) *Message {
	m.Payload = payload
	return m
}

func (m *Message) WithBlock1(bm *BlockMetadata) *Message {
	m.WithOption(OptBlock1, bm.Encode(), true)
	return m
}

func (m *Message) WithBlock2(bm *BlockMetadata) *Message {
	m.WithOption(OptBlock2, bm.Encode(), true)
	return m
}

func (m *Message) WithSize1(sz int) *Message {
	m.WithOption(OptSize1, sz, true)
	return m
}

func (m *Message) WithSize2(sz int) *Message {
	m.WithOption(OptSize2, sz, true)
	return m
}

func (m *Message) Accept() MediaType {
	opt := m.Option(OptAccept)
	if opt != nil {
		return opt.(MediaType)
	} else {
		return None
	}
}

func (m *Message) WithAccept(mt MediaType) *Message {
	if mt == None {
		return m
	}
	m.WithOption(OptAccept, mt, true)
	return m
}

func (m *Message) ContentFormat() MediaType {
	opt := m.Option(OptContentFormat)
	if opt != nil {
		return opt.(MediaType)
	} else {
		return None
	}
}

func (m *Message) WithContentFormat(mt MediaType) *Message {
	if mt == None {
		return m
	}
	m.WithOption(OptContentFormat, mt, true)
	return m
}

func (m *Message) WithCode(code COAPCode) *Message {
	m.Code = code
	return m
}

func (m *Message) WithLocationPath(s []string) *Message {
	m.WithOption(OptLocationPath, s, true)
	return m
}

func (m *Message) WithLocationPathString(path string) *Message {
	for path[0] == '/' {
		path = path[1:]
	}
	m.WithLocationPath(strings.Split(path, "/"))
	return m
}

// Path gets the Path set on this message if any.
func (m Message) LocationPath() []string {
	return m.optionStrings(OptLocationPath)
}

// PathString gets a path as a / separated string.
func (m Message) LocationPathString() string {
	return strings.Join(m.LocationPath(), "/")
}

func (m *Message) MakeReply(code COAPCode, payload []byte) *Message {
	rm := Message{}
	rm.Token = m.Token
	rm.MessageID = m.MessageID
	rm.Type = TypeAcknowledgement
	rm.Payload = payload
	rm.Code = code
	return &rm
}
