package postgres

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sevlumen/postgres/internal/pgwire"
)

func TestCleartextAuthenticationRejectedWithoutTLS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverResult := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			serverResult <- err
			return
		}
		defer connection.Close()
		if err := readUntypedMessage(connection); err != nil {
			serverResult <- err
			return
		}
		var auth pgwire.Buffer
		auth.Int32(3)
		if _, err := connection.Write(pgwire.Typed('R', auth.Bytes)); err != nil {
			serverResult <- err
			return
		}
		_ = connection.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		var value [1]byte
		_, err = connection.Read(value[:])
		if err == nil {
			serverResult <- errors.New("client sent a cleartext password")
			return
		}
		serverResult <- nil
	}()

	host, port := splitListenerAddress(t, listener.Addr())
	connector, err := NewConnector(Config{
		Host: host, Port: port, User: "identity", Password: "secret", Database: "identity",
		TLSMode: TLSDisable, ConnectTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = connector.Connect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "requires TLS") {
		t.Fatalf("expected TLS requirement error, got %v", err)
	}
	if err := <-serverResult; err != nil {
		t.Fatal(err)
	}
}

func TestMD5AuthenticationRejectedWithoutTLS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverResult := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			serverResult <- err
			return
		}
		defer connection.Close()
		if err := readUntypedMessage(connection); err != nil {
			serverResult <- err
			return
		}
		var auth pgwire.Buffer
		auth.Int32(5)
		auth.Raw([]byte{1, 2, 3, 4})
		if _, err := connection.Write(pgwire.Typed('R', auth.Bytes)); err != nil {
			serverResult <- err
			return
		}
		_ = connection.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		var value [1]byte
		_, err = connection.Read(value[:])
		if err == nil {
			serverResult <- errors.New("client sent an MD5 password response")
			return
		}
		serverResult <- nil
	}()

	host, port := splitListenerAddress(t, listener.Addr())
	connector, err := NewConnector(Config{
		Host: host, Port: port, User: "identity", Password: "secret", Database: "identity",
		TLSMode: TLSDisable, ConnectTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = connector.Connect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "MD5 password authentication requires TLS") {
		t.Fatalf("expected TLS requirement error, got %v", err)
	}
	if err := <-serverResult; err != nil {
		t.Fatal(err)
	}
}

func TestSCRAMRequiresVerifiedServerFinal(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverResult := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			serverResult <- err
			return
		}
		defer connection.Close()
		if err := readUntypedMessage(connection); err != nil {
			serverResult <- err
			return
		}
		var auth pgwire.Buffer
		auth.Int32(10)
		auth.CString("SCRAM-SHA-256")
		auth.Byte(0)
		if _, err := connection.Write(pgwire.Typed('R', auth.Bytes)); err != nil {
			serverResult <- err
			return
		}
		if err := readTypedMessage(connection); err != nil {
			serverResult <- err
			return
		}
		var ok pgwire.Buffer
		ok.Int32(0)
		_, err = connection.Write(pgwire.Typed('R', ok.Bytes))
		serverResult <- err
	}()

	host, port := splitListenerAddress(t, listener.Addr())
	connector, err := NewConnector(Config{
		Host: host, Port: port, User: "identity", Password: "secret", Database: "identity",
		TLSMode: TLSDisable, ConnectTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = connector.Connect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "without a verified server signature") {
		t.Fatalf("expected SCRAM final verification error, got %v", err)
	}
	if err := <-serverResult; err != nil {
		t.Fatal(err)
	}
}

func TestConnectTimeoutCoversStartup(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		_ = readUntypedMessage(connection)
		time.Sleep(time.Second)
	}()

	host, port := splitListenerAddress(t, listener.Addr())
	connector, err := NewConnector(Config{
		Host: host, Port: port, User: "identity", Password: "do-not-leak", Database: "identity",
		TLSMode: TLSDisable, ConnectTimeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err = connector.Connect(context.Background())
	if err == nil {
		t.Fatal("expected startup timeout")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("connect timeout took %v", elapsed)
	}
	if strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("error leaked password: %v", err)
	}
}

func readUntypedMessage(reader io.Reader) error {
	var lengthBytes [4]byte
	if _, err := io.ReadFull(reader, lengthBytes[:]); err != nil {
		return err
	}
	length := int(binary.BigEndian.Uint32(lengthBytes[:]))
	if length < 8 || length > 1<<20 {
		return errors.New("invalid startup message length")
	}
	_, err := io.CopyN(io.Discard, reader, int64(length-4))
	return err
}

func readTypedMessage(reader io.Reader) error {
	var header [5]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return err
	}
	length := int(binary.BigEndian.Uint32(header[1:]))
	if length < 4 || length > 1<<20 {
		return errors.New("invalid typed message length")
	}
	_, err := io.CopyN(io.Discard, reader, int64(length-4))
	return err
}

func splitListenerAddress(t *testing.T, address net.Addr) (string, uint16) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address.String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	return host, uint16(port)
}
