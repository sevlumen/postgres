package pgwire

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestTypedMessageRoundTrip(t *testing.T) {
	encoded := Typed('Q', []byte{'x', 0})
	message, err := ReadMessage(bytes.NewReader(encoded), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if message.Type != 'Q' || !bytes.Equal(message.Body, []byte{'x', 0}) {
		t.Fatalf("unexpected message: %#v", message)
	}
}

func TestReadMessageRejectsInvalidLength(t *testing.T) {
	data := []byte{'D', 0, 0, 0, 3}
	if _, err := ReadMessage(bytes.NewReader(data), 1024); err == nil {
		t.Fatal("expected invalid length")
	}
	data = []byte{'D', 0, 0, 4, 1}
	if _, err := ReadMessage(bytes.NewReader(data), 128); err == nil {
		t.Fatal("expected size limit")
	}
}

func TestExtendedQuerySeparatesParameters(t *testing.T) {
	payload, err := ExtendedQuery("SELECT $1", [][]byte{[]byte("payload') DROP TABLE users; --")}, []bool{false})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(payload, []byte("SELECT $1")) {
		t.Fatal("query missing")
	}
	if !bytes.Contains(payload, []byte("payload') DROP TABLE users; --")) {
		t.Fatal("argument missing")
	}
	if bytes.Contains(payload, []byte("SELECT payload")) {
		t.Fatal("argument changed SQL shape")
	}
}

func FuzzReadMessageNeverPanics(f *testing.F) {
	seed := append([]byte{'Z'}, make([]byte, 4)...)
	binary.BigEndian.PutUint32(seed[1:], 5)
	seed = append(seed, 'I')
	f.Add(seed)
	f.Fuzx(func(t *testing.T, data []byte) {
		_, _ = ReadMessage(bytes.NewReader(data), 1<<20)
	})
}

func TestExtendedQueryRejectsNULAndTooManyParameters(t *testing.T) {
	if _, err := ExtendedQuery("SELECT \x00", nil, nil); err == nil {
		t.Fatal("expected NUL query rejection")
	}
	params := make([][]byte, 32768)
	if _, err := ExtendedQuery("SELECT 1", params, nil); err == nil {
		t.Fatal("expected parameter count rejection")
	}
}
