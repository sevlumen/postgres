package pgwire

import (
	"encoding/binary"
	"errors"
	"fmt"
)

type Buffer struct{ Bytes []byte }

func (b *Buffer) Byte(value byte) { b.Bytes = append(b.Bytes, value) }
func (b *Buffer) Int16(value int16) {
	b.Bytes = binary.BigEndian.AppendUint16(b.Bytes, uint16(value))
}
func (b *Buffer) Int32(value int32) {
	b.Bytes = binary.BigEndian.AppendUint32(b.Bytes, uint32(value))
}
func (b *Buffer) Uint32(value uint32) {
	b.Bytes = binary.BigEndian.AppendUint32(b.Bytes, value)
}
func (b *Buffer) CString(value string) {
	b.Bytes = append(b.Bytes, value...)
	b.Bytes = append(b.Bytes, 0)
}
func (b *Buffer) Raw(value []byte) { b.Bytes = append(b.Bytes, value...) }

func Typed(messageType byte, body []byte) []byte {
	out := make([]byte, 0, 1+4+len(body))
	out = append(out, messageType)
	out = binary.BigEndian.AppendUint32(out, uint32(4+len(body)))
	out = append(out, body...)
	return out
}

func Untyped(code int32, body []byte) []byte {
	out := make([]byte, 0, 8+len(body))
	out = binary.BigEndian.AppendUint32(out, uint32(8+len(body)))
	out = binary.BigEndian.AppendUint32(out, uint32(code))
	out = append(out, body...)
	return out
}

type Cursor struct {
	data []byte
	pos  int
}

func NewCursor(data []byte) *Cursor { return &Cursor{data: data} }
func (c *Cursor) Remaining() int    { return len(c.data) - c.pos }
func (c *Cursor) Byte() (byte, error) {
	if c.Remaining() < 1 {
		return 0, errors.New("pgwire: truncated byte")
	}
	value := c.data[c.pos]
	c.pos++
	return value, nil
}
func (c *Cursor) Int16() (int16, error) {
	if c.Remaining() < 2 {
		return 0, errors.New("pgwire: truncated int16")
	}
	value := int16(binary.BigEndian.Uint16(c.data[c.pos : c.pos+2]))
	c.pos += 2
	return value, nil
}
func (c *Cursor) Int32() (int32, error) {
	if c.Remaining() < 4 {
		return 0, errors.New("pgwire: truncated int32")
	}
	value := int32(binary.BigEndian.Uint32(c.data[c.pos : c.pos+4]))
	c.pos += 4
	return value, nil
}
func (c *Cursor) Uint32() (uint32, error) {
	value, err := c.Int32()
	return uint32(value), err
}
func (c *Cursor) CString() (string, error) {
	start := c.pos
	for c.pos < len(c.data) && c.data[c.pos] != 0 {
		c.pos++
	}
	if c.pos >= len(c.data) {
		return "", errors.New("pgwire: unterminated string")
	}
	value := string(c.data[start:c.pos])
	c.pos++
	return value, nil
}
func (c *Cursor) Bytes(length int) ([]byte, error) {
	if length < 0 || c.Remaining() < length {
		return nil, fmt.Errorf("pgwire: truncated byte sequence: need %d, have %d", length, c.Remaining())
	}
	value := c.data[c.pos : c.pos+length]
	c.pos += length
	return value, nil
}
func (c *Cursor) Rest() []byte {
	value := c.data[c.pos:]
	c.pos = len(c.data)
	return value
}
