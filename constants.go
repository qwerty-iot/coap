// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package coap

import (
	"fmt"
	"strings"

	"github.com/qwerty-iot/tox"
)

// COAPType represents the message type.
type COAPType uint8

const (
	// Confirmable messages require acknowledgements.
	TypeConfirmable COAPType = 0
	// NonConfirmable messages do not require acknowledgements.
	TypeNonConfirmable COAPType = 1
	// Acknowledgement is a message indicating a response to confirmable message.
	TypeAcknowledgement COAPType = 2
	// Reset indicates a permanent negative acknowledgement.
	TypeReset COAPType = 3
)

var typeNames = [256]string{
	TypeConfirmable:     "Confirmable",
	TypeNonConfirmable:  "NonConfirmable",
	TypeAcknowledgement: "Acknowledgement",
	TypeReset:           "Reset",
}

func init() {
	for i := range typeNames {
		if typeNames[i] == "" {
			typeNames[i] = fmt.Sprintf("Unknown (0x%x)", i)
		}
	}
}

func (t COAPType) String() string {
	return typeNames[t]
}

// COAPCode is the type used for both request and response codes.
type COAPCode uint8

// Request Codes
const (
	CodeEmpty COAPCode = 0

	CodeGet    COAPCode = 1
	CodePost   COAPCode = 2
	CodePut    COAPCode = 3
	CodeDelete COAPCode = 4
	CodeFetch  COAPCode = 5
	CodePatch  COAPCode = 6
	CodeIPatch COAPCode = 7
)

// Response Codes
const (
	RspCodeCreated               COAPCode = 65
	RspCodeDeleted               COAPCode = 66
	RspCodeValid                 COAPCode = 67
	RspCodeChanged               COAPCode = 68
	RspCodeContent               COAPCode = 69
	RspCodeContinue              COAPCode = 95
	RspCodeBadRequest            COAPCode = 128
	RspCodeUnauthorized          COAPCode = 129
	RspCodeBadOption             COAPCode = 130
	RspCodeForbidden             COAPCode = 131
	RspCodeNotFound              COAPCode = 132
	RspCodeMethodNotAllowed      COAPCode = 133
	RspCodeNotAcceptable         COAPCode = 134
	RspCodePreconditionFailed    COAPCode = 140
	RspCodeRequestEntityTooLarge COAPCode = 141
	RspCodeUnsupportedMediaType  COAPCode = 143
	RspCodeInternalServerError   COAPCode = 160
	RspCodeNotImplemented        COAPCode = 161
	RspCodeBadGateway            COAPCode = 162
	RspCodeServiceUnavailable    COAPCode = 163
	RspCodeGatewayTimeout        COAPCode = 164
	RspCodeProxyingNotSupported  COAPCode = 165
)

var codeNames = [256]string{
	CodeGet:                      "GET",
	CodePost:                     "POST",
	CodePut:                      "PUT",
	CodeDelete:                   "DELETE",
	CodeFetch:                    "FETCH",
	CodePatch:                    "PATCH",
	CodeIPatch:                   "iPATCH",
	RspCodeCreated:               "Created",
	RspCodeDeleted:               "Deleted",
	RspCodeValid:                 "Valid",
	RspCodeChanged:               "Changed",
	RspCodeContent:               "Content",
	RspCodeContinue:              "Continue",
	RspCodeBadRequest:            "BadRequest",
	RspCodeUnauthorized:          "Unauthorized",
	RspCodeBadOption:             "BadOption",
	RspCodeForbidden:             "Forbidden",
	RspCodeNotFound:              "NotFound",
	RspCodeMethodNotAllowed:      "MethodNotAllowed",
	RspCodeNotAcceptable:         "NotAcceptable",
	RspCodePreconditionFailed:    "PreconditionFailed",
	RspCodeRequestEntityTooLarge: "RequestEntityTooLarge",
	RspCodeUnsupportedMediaType:  "UnsupportedMediaType",
	RspCodeInternalServerError:   "InternalServerError",
	RspCodeNotImplemented:        "NotImplemented",
	RspCodeBadGateway:            "BadGateway",
	RspCodeServiceUnavailable:    "ServiceUnavailable",
	RspCodeGatewayTimeout:        "GatewayTimeout",
	RspCodeProxyingNotSupported:  "ProxyingNotSupported",
}

func init() {
	for i := range codeNames {
		if codeNames[i] == "" {
			codeNames[i] = fmt.Sprintf("Unknown (0x%x)", i)
		}
	}
}

func ToCOAPCode(val string) COAPCode {
	ss := strings.Split(val, ".")
	if len(ss) != 2 {
		return RspCodeInternalServerError
	}
	return COAPCode(tox.ToInt(ss[0])<<5 | tox.ToInt(ss[1])&0x1F)
}

func (c COAPCode) String() string {
	return codeNames[c]
}

func (c COAPCode) NumberString() string {
	lower := c & 0x1F
	upper := c >> 5
	return fmt.Sprintf("%d.%02d", upper, lower)
}

// MediaType specifies the content type of a message.
type MediaType int

// Content types.
const (
	None          MediaType = -1
	TextPlain     MediaType = 0     // text/plain;charset=utf-8
	AppLinkFormat MediaType = 40    // application/link-format
	AppXML        MediaType = 41    // application/xml
	AppOctets     MediaType = 42    // application/octet-stream
	AppExi        MediaType = 47    // application/exi
	AppJSON       MediaType = 50    // application/json
	AppCBOR       MediaType = 60    // application/cbor
	AppSenmlCBOR  MediaType = 112   // application/senml_cbor
	AppLwm2mTLV   MediaType = 11542 //application/vnd.oma.lwm2m+tlv
	AppLwm2mJSON  MediaType = 11543 //application/vnd.oma.lwm2m+json
)
