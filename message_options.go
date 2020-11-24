package coap

import (
	"encoding/binary"
	fmt "fmt"
)

// OptionID identifies an option in a message.
type OptionID uint8

/*
   +-----+----+---+---+---+----------------+--------+--------+---------+
   | No. | C  | U | N | R | Name           | Format | Length | Default |
   +-----+----+---+---+---+----------------+--------+--------+---------+
   |   1 | x  |   |   | x | If-Match       | opaque | 0-8    | (none)  |
   |   3 | x  | x | - |   | Uri-Host       | string | 1-255  | (see    |
   |     |    |   |   |   |                |        |        | below)  |
   |   4 |    |   |   | x | ETag           | opaque | 1-8    | (none)  |
   |   5 | x  |   |   |   | If-None-Match  | empty  | 0      | (none)  |
   |   7 | x  | x | - |   | Uri-Port       | uint   | 0-2    | (see    |
   |     |    |   |   |   |                |        |        | below)  |
   |   8 |    |   |   | x | Location-Path  | string | 0-255  | (none)  |
   |  11 | x  | x | - | x | Uri-Path       | string | 0-255  | (none)  |
   |  12 |    |   |   |   | Content-Format | uint   | 0-2    | (none)  |
   |  14 |    | x | - |   | Max-Age        | uint   | 0-4    | 60      |
   |  15 | x  | x | - | x | Uri-Query      | string | 0-255  | (none)  |
   |  17 | x  |   |   |   | Accept         | uint   | 0-2    | (none)  |
   |  20 |    |   |   | x | Location-Query | string | 0-255  | (none)  |
   |  23 | x  | x |   |   | Block2         | uint   | 0-3    | (none)  |
   |  27 | x  | x |   |   | Block1         | uint   | 0-3    | (none)  |
   |  35 | x  | x | - |   | Proxy-Uri      | string | 1-1034 | (none)  |
   |  39 | x  | x | - |   | Proxy-Scheme   | string | 1-255  | (none)  |
   |  60 |    |   | x |   | Size1          | uint   | 0-4    | (none)  |
   +-----+----+---+---+---+----------------+--------+--------+---------+
*/

// Option IDs.
const (
	OptIfMatch       OptionID = 1
	OptURIHost       OptionID = 3
	OptETag          OptionID = 4
	OptIfNoneMatch   OptionID = 5
	OptObserve       OptionID = 6
	OptURIPort       OptionID = 7
	OptLocationPath  OptionID = 8
	OptURIPath       OptionID = 11
	OptContentFormat OptionID = 12
	OptMaxAge        OptionID = 14
	OptURIQuery      OptionID = 15
	OptAccept        OptionID = 17
	OptLocationQuery OptionID = 20
	OptBlock2        OptionID = 23
	OptBlock1        OptionID = 27
	OptProxyURI      OptionID = 35
	OptProxyScheme   OptionID = 39
	OptSize1         OptionID = 60
)

// Option value format (RFC7252 section 3.2)
type valueFormat uint8

const (
	valueUnknown valueFormat = iota
	valueEmpty
	valueOpaque
	valueUint
	valueString
)

type optionDef struct {
	valueFormat valueFormat
	minLen      int
	maxLen      int
}

var optionDefs = [256]optionDef{
	OptIfMatch:       {valueFormat: valueOpaque, minLen: 0, maxLen: 8},
	OptURIHost:       {valueFormat: valueString, minLen: 1, maxLen: 255},
	OptETag:          {valueFormat: valueOpaque, minLen: 1, maxLen: 8},
	OptIfNoneMatch:   {valueFormat: valueEmpty, minLen: 0, maxLen: 0},
	OptObserve:       {valueFormat: valueUint, minLen: 0, maxLen: 3},
	OptURIPort:       {valueFormat: valueUint, minLen: 0, maxLen: 2},
	OptLocationPath:  {valueFormat: valueString, minLen: 0, maxLen: 255},
	OptURIPath:       {valueFormat: valueString, minLen: 0, maxLen: 255},
	OptContentFormat: {valueFormat: valueUint, minLen: 0, maxLen: 2},
	OptMaxAge:        {valueFormat: valueUint, minLen: 0, maxLen: 4},
	OptURIQuery:      {valueFormat: valueString, minLen: 0, maxLen: 255},
	OptAccept:        {valueFormat: valueUint, minLen: 0, maxLen: 2},
	OptLocationQuery: {valueFormat: valueString, minLen: 0, maxLen: 255},
	OptProxyURI:      {valueFormat: valueString, minLen: 1, maxLen: 1034},
	OptProxyScheme:   {valueFormat: valueString, minLen: 1, maxLen: 255},
	OptSize1:         {valueFormat: valueUint, minLen: 0, maxLen: 4},
	OptBlock2:        {valueFormat: valueOpaque, minLen: 0, maxLen: 3},
}

type option struct {
	ID    OptionID
	Value interface{}
}

func encodeInt(v uint32) []byte {
	switch {
	case v == 0:
		return nil
	case v < 256:
		return []byte{byte(v)}
	case v < 65536:
		rv := []byte{0, 0}
		binary.BigEndian.PutUint16(rv, uint16(v))
		return rv
	case v < 16777216:
		rv := []byte{0, 0, 0, 0}
		binary.BigEndian.PutUint32(rv, uint32(v))
		return rv[1:]
	default:
		rv := []byte{0, 0, 0, 0}
		binary.BigEndian.PutUint32(rv, uint32(v))
		return rv
	}
}

func decodeInt(b []byte) uint32 {
	tmp := []byte{0, 0, 0, 0}
	copy(tmp[4-len(b):], b)
	return binary.BigEndian.Uint32(tmp)
}

func (o option) toBytes() []byte {
	var v uint32

	switch i := o.Value.(type) {
	case string:
		return []byte(i)
	case []byte:
		return i
	case MediaType:
		v = uint32(i)
	case int:
		v = uint32(i)
	case int32:
		v = uint32(i)
	case uint:
		v = uint32(i)
	case uint32:
		v = i
	default:
		panic(fmt.Errorf("coap: invalid type for option %x: %T (%v)",
			o.ID, o.Value, o.Value))
	}

	return encodeInt(v)
}

func parseOptionValue(optionID OptionID, valueBuf []byte) interface{} {
	def := optionDefs[optionID]
	if def.valueFormat == valueUnknown {
		// Skip unrecognized options (RFC7252 section 5.4.1)
		return nil
	}
	if len(valueBuf) < def.minLen || len(valueBuf) > def.maxLen {
		// Skip options with illegal value length (RFC7252 section 5.4.3)
		return nil
	}
	switch def.valueFormat {
	case valueUint:
		intValue := decodeInt(valueBuf)
		if optionID == OptContentFormat || optionID == OptAccept {
			return MediaType(intValue)
		} else {
			return intValue
		}
	case valueString:
		return string(valueBuf)
	case valueOpaque, valueEmpty:
		return valueBuf
	}
	// Skip unrecognized options (should never be reached)
	return nil
}

type options []option

func (o options) Len() int {
	return len(o)
}

func (o options) Less(i, j int) bool {
	if o[i].ID == o[j].ID {
		return i < j
	}
	return o[i].ID < o[j].ID
}

func (o options) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}

func (o options) Minus(oid OptionID) options {
	rv := options{}
	for _, opt := range o {
		if opt.ID != oid {
			rv = append(rv, opt)
		}
	}
	return rv
}
