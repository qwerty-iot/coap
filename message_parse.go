package coap

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sort"
)

const (
	extoptByteCode   = 13
	extoptByteAddend = 13
	extoptWordCode   = 14
	extoptWordAddend = 269
	extoptError      = 15
)

func (m *Message) headerSize() int {
	tmpbuf := []byte{0, 0}
	binary.BigEndian.PutUint16(tmpbuf, m.MessageID)

	/*
	     0                   1                   2                   3
	    0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |Ver| T |  TKL  |      Code     |          Message ID           |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |   Token (if any, TKL bytes) ...
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |   Options (if any) ...
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |1 1 1 1 1 1 1 1|    Payload (if any) ...
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	*/

	buf := bytes.Buffer{}
	buf.Write([]byte{
		(1 << 6) | (uint8(m.Type) << 4) | uint8(0xf&len(m.Token)),
		byte(m.Code),
		tmpbuf[0], tmpbuf[1],
	})
	buf.Write(m.Token)

	/*
	     0   1   2   3   4   5   6   7
	   +---------------+---------------+
	   |               |               |
	   |  Option Delta | Option Length |   1 byte
	   |               |               |
	   +---------------+---------------+
	   \                               \
	   /         Option Delta          /   0-2 bytes
	   \          (extended)           \
	   +-------------------------------+
	   \                               \
	   /         Option Length         /   0-2 bytes
	   \          (extended)           \
	   +-------------------------------+
	   \                               \
	   /                               /
	   \                               \
	   /         Option Value          /   0 or more bytes
	   \                               \
	   /                               /
	   \                               \
	   +-------------------------------+

	   See parseExtOption(), extendOption()
	   and writeOptionHeader() below for implementation details
	*/

	extendOpt := func(opt int) (int, int) {
		ext := 0
		if opt >= extoptByteAddend {
			if opt >= extoptWordAddend {
				ext = opt - extoptWordAddend
				opt = extoptWordCode
			} else {
				ext = opt - extoptByteAddend
				opt = extoptByteCode
			}
		}
		return opt, ext
	}

	writeOptHeader := func(delta, length int) {
		d, dx := extendOpt(delta)
		l, lx := extendOpt(length)

		buf.WriteByte(byte(d<<4) | byte(l))

		tmp := []byte{0, 0}
		writeExt := func(opt, ext int) {
			switch opt {
			case extoptByteCode:
				buf.WriteByte(byte(ext))
			case extoptWordCode:
				binary.BigEndian.PutUint16(tmp, uint16(ext))
				buf.Write(tmp)
			}
		}

		writeExt(d, dx)
		writeExt(l, lx)
	}

	sort.Stable(&m.opts)

	prev := 0

	for _, o := range m.opts {
		b := o.toBytes()
		writeOptHeader(int(o.ID)-prev, len(b))
		buf.Write(b)
		prev = int(o.ID)
	}

	if len(m.Payload) > 0 {
		buf.Write([]byte{0xff})
	}

	return buf.Len()
}

// marshalBinary produces the binary form of this Message.
func (m *Message) marshalBinary() ([]byte, error) {
	tmpbuf := []byte{0, 0}
	binary.BigEndian.PutUint16(tmpbuf, m.MessageID)

	/*
	     0                   1                   2                   3
	    0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |Ver| T |  TKL  |      Code     |          Message ID           |
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |   Token (if any, TKL bytes) ...
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |   Options (if any) ...
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	   |1 1 1 1 1 1 1 1|    Payload (if any) ...
	   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	*/

	buf := bytes.Buffer{}
	buf.Write([]byte{
		(1 << 6) | (uint8(m.Type) << 4) | uint8(0xf&len(m.Token)),
		byte(m.Code),
		tmpbuf[0], tmpbuf[1],
	})
	buf.Write(m.Token)

	/*
	     0   1   2   3   4   5   6   7
	   +---------------+---------------+
	   |               |               |
	   |  Option Delta | Option Length |   1 byte
	   |               |               |
	   +---------------+---------------+
	   \                               \
	   /         Option Delta          /   0-2 bytes
	   \          (extended)           \
	   +-------------------------------+
	   \                               \
	   /         Option Length         /   0-2 bytes
	   \          (extended)           \
	   +-------------------------------+
	   \                               \
	   /                               /
	   \                               \
	   /         Option Value          /   0 or more bytes
	   \                               \
	   /                               /
	   \                               \
	   +-------------------------------+

	   See parseExtOption(), extendOption()
	   and writeOptionHeader() below for implementation details
	*/

	extendOpt := func(opt int) (int, int) {
		ext := 0
		if opt >= extoptByteAddend {
			if opt >= extoptWordAddend {
				ext = opt - extoptWordAddend
				opt = extoptWordCode
			} else {
				ext = opt - extoptByteAddend
				opt = extoptByteCode
			}
		}
		return opt, ext
	}

	writeOptHeader := func(delta, length int) {
		d, dx := extendOpt(delta)
		l, lx := extendOpt(length)

		buf.WriteByte(byte(d<<4) | byte(l))

		tmp := []byte{0, 0}
		writeExt := func(opt, ext int) {
			switch opt {
			case extoptByteCode:
				buf.WriteByte(byte(ext))
			case extoptWordCode:
				binary.BigEndian.PutUint16(tmp, uint16(ext))
				buf.Write(tmp)
			}
		}

		writeExt(d, dx)
		writeExt(l, lx)
	}

	sort.Stable(&m.opts)

	prev := 0

	for _, o := range m.opts {
		b := o.toBytes()
		writeOptHeader(int(o.ID)-prev, len(b))
		buf.Write(b)
		prev = int(o.ID)
	}

	if len(m.Payload) > 0 {
		buf.Write([]byte{0xff})
	}

	buf.Write(m.Payload)

	return buf.Bytes(), nil
}

// parseMessage extracts the Message from the given input.
func parseMessage(data []byte) (Message, error) {
	rv := Message{}
	return rv, rv.unmarshalBinary(data)
}

// unmarshalBinary parses the given binary slice as a Message.
func (m *Message) unmarshalBinary(data []byte) error {
	m.packetSize = len(data)
	if len(data) < 4 {
		return errors.New("coap: short packet")
	}

	if data[0]>>6 != 1 {
		return errors.New("coap: invalid version")
	}

	m.Type = COAPType((data[0] >> 4) & 0x3)
	tokenLen := int(data[0] & 0xf)
	if tokenLen > 8 {
		return ErrInvalidTokenLen
	}

	m.Code = COAPCode(data[1])
	m.MessageID = binary.BigEndian.Uint16(data[2:4])

	if tokenLen > 0 {
		m.Token = make([]byte, tokenLen)
	}
	if len(data) < 4+tokenLen {
		return errors.New("coap: truncated")
	}
	copy(m.Token, data[4:4+tokenLen])
	b := data[4+tokenLen:]
	prev := 0

	parseExtOpt := func(opt int) (int, error) {
		switch opt {
		case extoptByteCode:
			if len(b) < 1 {
				return -1, errors.New("coap: truncated")
			}
			opt = int(b[0]) + extoptByteAddend
			b = b[1:]
		case extoptWordCode:
			if len(b) < 2 {
				return -1, errors.New("coap: truncated")
			}
			opt = int(binary.BigEndian.Uint16(b[:2])) + extoptWordAddend
			b = b[2:]
		}
		return opt, nil
	}

	for len(b) > 0 {
		if b[0] == 0xff {
			b = b[1:]
			break
		}

		delta := int(b[0] >> 4)
		length := int(b[0] & 0x0f)

		if delta == extoptError || length == extoptError {
			return errors.New("coap: unexpected extended option marker")
		}

		b = b[1:]

		delta, err := parseExtOpt(delta)
		if err != nil {
			return err
		}
		length, err = parseExtOpt(length)
		if err != nil {
			return err
		}

		if len(b) < length {
			return errors.New("coap: truncated")
		}

		oid := OptionID(prev + delta)
		opval := parseOptionValue(oid, b[:length])
		b = b[length:]
		prev = int(oid)

		if opval != nil {
			m.opts = append(m.opts, option{ID: oid, Value: opval})
		}
	}
	m.Payload = b
	return nil
}
