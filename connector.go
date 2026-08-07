package postgres

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/sevlumen/postgres/internal/pgwire"
	"github.com/sevlumen/postgres/internal/scram"
)

// Connector implements database/sql/driver.Connector.
type Connector struct{ config Config }

func NewConnector(config Config) (*Connector, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Connector{config: config}, nil
}

func Open(dsn string) (*sql.DB, error) {
	config, err := ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	return OpenConfig(config)
}

func OpenConfig(config Config) (*sql.DB, error) {
	connector, err := NewConnector(config)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(connector), nil
}

func (c *Connector) Driver() driver.Driver { return connectorDriver{} }
func (c *Connector) Connect(ctx context.Context) (driver.Conn, error) {
	return connect(ctx, c.config)
}

type connectorDriver struct{}

func (connectorDriver) Open(name string) (driver.Conn, error) {
	config, err := ParseConfig(name)
	if err != nil {
		return nil, err
	}
	return connect(context.Background(), config)
}

func connect(ctx context.Context, config Config) (*conn, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	connectCtx, cancelConnect := context.WithTimeout(ctx, config.ConnectTimeout)
	defer cancelConnect()

	dialer := net.Dialer{Timeout: config.ConnectTimeout, KeepAlive: 30 * time.Second}
	networkConn, err := dialer.DialContext(connectCtx, "tcp", config.address())
	if err != nil {
		return nil, fmt.Errorf("postgres: dial %s: %w", config.address(), err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = networkConn.Close()
		}
	}()

	// The connect timeout covers the complete handshake, including the
	// PostgreSQL SSLRequest response. A TCP peer that accepts but never replies
	// must not be able to hold a connection attempt indefinitely.
	if deadline, ok := connectCtx.Deadline(); ok {
		_ = networkConn.SetDeadline(deadline)
	}

	secure := false
	if config.TLSMode != TLSDisable {
		if _, err := networkConn.Write(pgwire.Untyped(pgwire.SSLRequestCode, nil)); err != nil {
			return nil, fmt.Errorf("postgres: send SSLRequest: %w", err)
		}
		var response [1]byte
		if _, err := io.ReadFull(networkConn, response[:]); err != nil {
			return nil, fmt.Errorf("postgres: read SSL response: %w", err)
		}
		switch response[0] {
		case 'S':
			tlsConfig, err := buildTLSConfig(config)
			if err != nil {
				return nil, err
			}
			tlsConn := tls.Client(networkConn, tlsConfig)
			if err := tlsConn.HandshakeContext(connectCtx); err != nil {
				return nil, fmt.Errorf("postgres: TLS handshake: %w", err)
			}
			networkConn = tlsConn
			secure = true
		case 'N':
			return nil, fmt.Errorf("postgres: server rejected TLS for sslmode=%s", config.TLSMode)
		default:
			return nil, fmt.Errorf("postgres: unexpected SSL response %q", response[0])
		}
	}

	connection := newConn(networkConn, config, secure)
	if err := connection.startup(connectCtx); err != nil {
		return nil, err
	}
	if err := networkConn.SetDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("postgres: clear startup deadline: %w", err)
	}
	cleanup = false
	return connection, nil
}

func buildTLSConfig(config Config) (*tls.Config, error) {
	var tlsConfig *tls.Config
	if config.TLSConfig != nil {
		tlsConfig = config.TLSConfig.Clone()
	} else {
		tlsConfig = &tls.Config{}
	}
	if tlsConfig.MinVersion == 0 || tlsConfig.MinVersion < tls.VersionTLS12 {
		tlsConfig.MinVersion = tls.VersionTLS12
	}
	if tlsConfig.ServerName == "" {
		tlsConfig.ServerName = config.ServerName
	}
	if config.RootCAs != nil {
		tlsConfig.RootCAs = config.RootCAs
	}

	switch config.TLSMode {
	case TLSRequire:
		tlsConfig.InsecureSkipVerify = true // encryption without identity verification by definition
		tlsConfig.VerifyConnection = nil
	case TLSVerifyCA:
		tlsConfig.InsecureSkipVerify = true
		tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("postgres: TLS peer sent no certificate")
			}
			roots := tlsConfig.RootCAs
			if roots == nil {
				var err error
				roots, err = x509.SystemCertPool()
				if err != nil {
					return fmt.Errorf("postgres: load system roots: %w", err)
				}
			}
			intermediates := x509.NewCertPool()
			for _, cert := range state.PeerCertificates[1:] {
				intermediates.AddCert(cert)
			}
			_, err := state.PeerCertificates[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates})
			return err
		}
	case TLSVerifyFull:
		tlsConfig.InsecureSkipVerify = false
	case TLSDisable:
		return nil, errors.New("postgres: internal error: TLS config requested for disabled TLS")
	}
	return tlsConfig, nil
}

func (c *conn) startup(ctx context.Context) error {
	if err := c.write(ctx, pgwire.Startup(c.config.User, c.config.Database, c.config.ApplicationName, c.config.RuntimeParams)); err != nil {
		return fmt.Errorf("postgres: send startup message: %w", err)
	}
	var scramClient *scram.Client
	scramFinalVerified := false
	authenticated := false
	for {
		message, err := c.read()
		if err != nil {
			return fmt.Errorf("postgres: startup read: %w", err)
		}
		switch message.Type {
		case 'R':
			cursor := pgwire.NewCursor(message.Body)
			code, err := cursor.Int32()
			if err != nil {
				return err
			}
			switch code {
			case 0:
				if scramClient != nil && !scramFinalVerified {
					return errors.New("postgres: SCRAM authentication completed without a verified server signature")
				}
				authenticated = true
			case 3:
				if !c.secure && !c.config.AllowInsecureAuthentication {
					return errors.New("postgres: cleartext password authentication requires TLS")
				}
				if err := c.write(ctx, pgwire.Password(c.config.Password)); err != nil {
					return err
				}
			case 5:
				if !c.secure && !c.config.AllowInsecureAuthentication {
					return errors.New("postgres: MD5 password authentication requires TLS")
				}
				salt, err := cursor.Bytes(4)
				if err != nil {
					return err
				}
				digest1 := md5.Sum([]byte(c.config.Password + c.config.User))
				digest2 := md5.Sum(append([]byte(hex.EncodeToString(digest1[:])), salt...))
				if err := c.write(ctx, pgwire.Password("md5"+hex.EncodeToString(digest2[:]))); err != nil {
					return err
				}
			case 10:
				if scramClient != nil || authenticated {
					return errors.New("postgres: unexpected SCRAM authentication restart")
				}
				mechanisms, err := parseMechanisms(cursor.Rest())
				if err != nil {
					return err
				}
				if !contains(mechanisms, "SCRAM-SHA-256") {
					return fmt.Errorf("postgres: server does not offer SCRAM-SHA-256: %v", mechanisms)
				}
				scramClient, err = scram.New(c.config.User, c.config.Password)
				if err != nil {
					return err
				}
				if err := c.write(ctx, pgwire.SASLInitial(scramClient.Mechanism(), scramClient.FirstMessage())); err != nil {
					return err
				}
			case 11:
				if scramClient == nil {
					return errors.New("postgres: unexpected SCRAM continuation")
				}
				response, err := scramClient.Continue(string(cursor.Rest()))
				if err != nil {
					return err
				}
				if err := c.write(ctx, pgwire.SASLResponse(response)); err != nil {
					return err
				}
			case 12:
				if scramClient == nil || scramFinalVerified {
					return errors.New("postgres: unexpected SCRAM final message")
				}
				if err := scramClient.Final(string(cursor.Rest())); err != nil {
					return err
				}
				scramFinalVerified = true
			default:
				return fmt.Errorf("postgres: unsupported authentication request %d", code)
			}
		case 'S':
			name, value, err := parseParameterStatus(message.Body)
			if err != nil {
				return err
			}
			c.parameters[name] = value
		case 'K':
			cursor := pgwire.NewCursor(message.Body)
			pid, err := cursor.Int32()
			if err != nil {
				return err
			}
			secret := append([]byte(nil), cursor.Rest()...)
			if len(secret) != 4 {
				return fmt.Errorf("postgres: protocol 3.0 expected 4-byte cancel key, got %d", len(secret))
			}
			c.backendPID, c.cancelKey = pid, secret
		case 'E':
			return parseError(message.Body)
		case 'N':
			c.lastNotice = parseError(message.Body)
		case 'Z':
			if !authenticated {
				return errors.New("postgres: server became ready before authentication completed")
			}
			if len(message.Body) != 1 {
				return errors.New("postgres: malformed ReadyForQuery")
			}
			c.txStatus = message.Body[0]
			return nil
		case 'v':
			// Protocol 3.0 is understood by all supported PostgreSQL versions.
		default:
			return fmt.Errorf("postgres: unexpected startup message %q", message.Type)
		}
	}
}

func parseMechanisms(body []byte) ([]string, error) {
	cursor := pgwire.NewCursor(body)
	var values []string
	for cursor.Remaining() > 0 {
		value, err := cursor.CString()
		if err != nil {
			return nil, err
		}
		if value == "" {
			break
		}
		values = append(values, value)
	}
	return values, nil
}
func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
func parseParameterStatus(body []byte) (string, string, error) {
	cursor := pgwire.NewCursor(body)
	name, err := cursor.CString()
	if err != nil {
		return "", "", err
	}
	value, err := cursor.CString()
	if err != nil {
		return "", "", err
	}
	return name, value, nil
}
