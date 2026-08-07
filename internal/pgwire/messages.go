package pgwire

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
)

const (
	Protocol30        int32 = 196608
	SSLRequestCode    int32 = 80877103
	CancelRequestCode int32 = 80877102
)

type Message struct {
	Type byte
	Body []byte
}

func ReadMessage(reader io.Reader, maxSize int32) (Message, error) {
	var header [5]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return Message{}, err
	}
	length := int32(binary.BigEndian.Uint32(header[1:]))
	if length < 4 {
		return Message{}, fmt.Errorf("pgwire: invalid message length %d", length)
	}
	if maxSize > 0 && length > maxSize {
		return Message{}, fmt.Errorf("pgwire: message length %d exceeds limit %d", length, maxSize)
	}
	body := make([]byte, int(length)-4)
	if _, err := io.ReadFull(reader, body); err != nil {
		return Message{}, err
	}
	return Message{Type: header[0], Body: body}, nil
}

func Startup(user, database, application string, params map[string]string) []byte {
	var body Buffer
	body.CString("user")
	body.CString(user)
	body.CString("database")
	body.CString(database)
	// Stable text output is part of the v1 codec contract.
	body.CString("client_encoding")
	body.CString("UTF8")
	body.CString("DateStyle")
	body.CString("ISO")
	body.CString("bytea_output")
	body.CString("hex")
	if application != "" {
		body.CString("application_name")
		body.CString(application)
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := params[key]
		switch key {
		case "user", "database", "password", "client_encoding", "DateStyle", "bytea_output", "application_name":
			continue
		}
		body.CString(key)
		body.CString(value)
	}
	body.Byte(0)
	return Untyped(Protocol30, body.Bytes)
}

func Query(sql string) []byte {
	var body Buffer
	body.CString(sql)
	return Typed('Q', body.Bytes)
}

func ExtendedQuery(sql string, params [][]byte, nulls []bool) ([]byte, error) {
	if strings.IndexByte(sql, 0) >= 0 {
		return nil, errors.New("pgwire: query contains a NUL byte")
	}
	if len(params) > math.MaxInt16 {
		return nil, fmt.Errorf("pgwire: too many parameters: %d", len(params))
	}
	var out []byte
	var parse Buffer
	parse.CString("")
	parse.CString(sql)
	parse.Int16(0)
	out = append(out, Typed('P', parse.Bytes)...)

	var bind Buffer
	bind.CString("")
	bind.CString("")
	bind.Int16(0)
	bind.Int16(int16(len(params)))
	for index, value := range params {
		if index < len(nulls) && nulls[index] {
			bind.Int32(-1)
			continue
		}
		if len(value) > math.MaxInt32 {
			return nil, fmt.Errorf("pgwire: parameter %d is too large", index+1)
		}
		bind.Int32(int32(len(value)))
		bind.Raw(value)
	}
	bind.Int16(0)
	out = append(out, Typed('B', bind.Bytes)...)

	var describe Buffer
	describe.Byte('P')
	describe.CString("")
	out = append(out, Typed('D', describe.Bytes)...)

	var execute Buffer
	execute.CString("")
	execute.Int32(0)
	out = append(out, Typed('E', execute.Bytes)...)
	out = append(out, Typed('S', nil)...)
	return out, nil
}

func SASLInitial(mechanism, initial string) []byte {
	var body Buffer
	body.CString(mechanism)
	body.Int32(int32(len(initial)))
	body.Raw([]byte(initial))
	return Typed('p', body.Bytes)
}

func SASLResponse(value string) []byte { return Typed('p', []byte(value)) }
func Password(value string) []byte {
	var body Buffer
	body.CString(value)
	return Typed('p', body.Bytes)
}
func Terminate() []byte { return Typed('X', nil) }

func CancelRequest(pid int32, secret []byte) []byte {
	var body Buffer
	body.Int32(pid)
	body.Raw(secret)
	return Untyped(CancelRequestCode, body.Bytes)
}
