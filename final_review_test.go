package postgres

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sevlumen/postgres/internal/pgwire"
)

func TestConnectTimeoutCoversSSLNegotiation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		_ = readUntypedMessage(connection)
		time.Sleep(time.Second)
	}()

	host, port := splitListenerAddress(t, listener.Addr())
	connector, err := NewConnector(Config{
		Host: host, Port: port, User: "identity", Password: "do-not-leak", Database: "identity",
		TLSMode: TLSRequire, ConnectTimeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	_, err = connector.Connect(context.Background())
	if err == nil {
		t.Fatal("expected SSL negotiation timeout")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("SSL negotiation timeout took %v", elapsed)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("expected timeout, got %v", err)
	}
	<-serverDone
}

func TestBuildTLSConfigPreservesStricterMinimum(t *testing.T) {
	config := Config{
		User:      "identity",
		TLSMode:   TLSRequire,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13},
	}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}

	tlsConfig, err := buildTLSConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if tlsConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("MinVersion=%d want TLS 1.3", tlsConfig.MinVersion)
	}
}

func TestDatabaseSQLDoesNotRetryUnknownOutcome(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	var executions atomic.Int32
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			serveUnknownOutcome(connection, &executions)
		}
	}()

	host, port := splitListenerAddress(t, listener.Addr())
	db, err := OpenConfig(Config{
		Host: host, Port: port, User: "identity", Database: "identity",
		TLSMode: TLSDisable, ConnectTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, `INSERT INTO audit_log(message) VALUES($1)`, "once")
	if !errors.Is(err, ErrOperationOutcomeUnknown) {
		t.Fatalf("expected unknown-outcome error, got %v", err)
	}

	_ = listener.Close()
	<-serverDone
	if got := executions.Load(); got != 1 {
		t.Fatalf("database/sql retried an ambiguous operation %d times", got)
	}
}

func serveUnknownOutcome(connection net.Conn, executions *atomic.Int32) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
	if err := readUntypedMessage(connection); err != nil {
		return
	}

	var auth pgwire.Buffer
	auth.Int32(0)
	if _, err := connection.Write(pgwire.Typed('R', auth.Bytes)); err != nil {
		return
	}
	var backendKey pgwire.Buffer
	backendKey.Int32(1234)
	backendKey.Raw([]byte{1, 2, 3, 4})
	if _, err := connection.Write(pgwire.Typed('K', backendKey.Bytes)); err != nil {
		return
	}
	if _, err := connection.Write(pgwire.Typed('Z', []byte{'I'})); err != nil {
		return
	}

	for {
		message, err := pgwire.ReadMessage(connection, 1<<20)
		if err != nil {
			return
		}
		if message.Type == 'E' {
			executions.Add(1)
		}
		if message.Type == 'S' {
			// Close without a response after Execute reached the server. The
			// operation outcome is ambiguous and must never be retried.
			return
		}
	}
}
