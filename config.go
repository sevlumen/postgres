package postgres

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TLSMode controls PostgreSQL transport encryption and certificate checks.
type TLSMode string

const (
	TLSDisable    TLSMode = "disable"
	TLSRequire    TLSMode = "require"
	TLSVerifyCA   TLSMode = "verify-ca"
	TLSVerifyFull TLSMode = "verify-full"
)

const (
	defaultPort           = 5432
	defaultConnectTimeout = 10 * time.Second
	defaultCancelTimeout  = 5 * time.Second
	defaultMaxMessageSize = 64 << 20
)

// Config contains connection settings for one PostgreSQL server.
type Config struct {
	Host       string
	Port       uint16
	User       string
	Password   string
	Database   string
	TLSMode    TLSMode
	TLSConfig  *tls.Config
	RootCAs    *x509.CertPool
	ServerName string

	ConnectTimeout  time.Duration
	CancelTimeout   time.Duration
	MaxMessageSize  int32
	ApplicationName string
	RuntimeParams   map[string]string

	// AllowInsecureAuthentication permits cleartext password authentication on
	// an unencrypted transport. It is false by default.
	AllowInsecureAuthentication bool
}

// ParseConfig parses a postgres:// or postgresql:// URL.
func ParseConfig(dsn string) (Config, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		// Never include the original DSN or parser error here: either may
		// contain credentials from the URL authority.
		return Config{}, errors.New("postgres: invalid DSN")
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return Config{}, fmt.Errorf("postgres: unsupported DSN scheme %q", u.Scheme)
	}
	if u.Fragment != "" {
		return Config{}, errors.New("postgres: DSN fragments are not supported")
	}
	cfg := Config{
		Host:           u.Hostname(),
		Port:           defaultPort,
		TLSMode:        TLSVerifyFull,
		ConnectTimeout: defaultConnectTimeout,
		CancelTimeout:  defaultCancelTimeout,
		MaxMessageSize: defaultMaxMessageSize,
		RuntimeParams:  make(map[string]string),
	}
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if port := u.Port(); port != "" {
		value, parseErr := strconv.ParseUint(port, 10, 16)
		if parseErr != nil || value == 0 {
			return Config{}, fmt.Errorf("postgres: invalid port %q", port)
		}
		cfg.Port = uint16(value)
	}
	if u.User != nil {
		cfg.User = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}
	cfg.Database = strings.TrimPrefix(u.EscapedPath(), "/")
	if decoded, decodeErr := url.PathUnescape(cfg.Database); decodeErr == nil {
		cfg.Database = decoded
	}
	if cfg.Database == "" {
		cfg.Database = cfg.User
	}

	query := u.Query()
	if value := query.Get("sslmode"); value != "" {
		cfg.TLSMode = TLSMode(value)
	}
	if value := query.Get("connect_timeout"); value != "" {
		seconds, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil || seconds <= 0 {
			return Config{}, fmt.Errorf("postgres: invalid connect_timeout %q", value)
		}
		cfg.ConnectTimeout = time.Duration(seconds) * time.Second
	}
	cfg.ApplicationName = query.Get("application_name")
	if value := query.Get("server_name"); value != "" {
		cfg.ServerName = value
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate normalizes defaults and rejects insecure or incomplete settings.
func (c *Config) Validate() error {
	if c.Host == "" {
		c.Host = "localhost"
	}
	if c.Port == 0 {
		c.Port = defaultPort
	}
	if c.User == "" {
		return errors.New("postgres: user is required")
	}
	if c.Database == "" {
		c.Database = c.User
	}
	if c.TLSMode == "" {
		c.TLSMode = TLSVerifyFull
	}
	switch c.TLSMode {
	case TLSDisable, TLSRequire, TLSVerifyCA, TLSVerifyFull:
	default:
		return fmt.Errorf("postgres: unsupported sslmode %q", c.TLSMode)
	}
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = defaultConnectTimeout
	}
	if c.CancelTimeout <= 0 {
		c.CancelTimeout = defaultCancelTimeout
	}
	if c.MaxMessageSize <= 0 {
		c.MaxMessageSize = defaultMaxMessageSize
	}
	if c.MaxMessageSize < 1024 {
		return errors.New("postgres: max message size must be at least 1024 bytes")
	}
	if c.ServerName == "" {
		c.ServerName = c.Host
	}
	if c.RuntimeParams == nil {
		c.RuntimeParams = make(map[string]string)
	}
	for name, value := range map[string]string{
		"host": c.Host, "user": c.User, "password": c.Password,
		"database": c.Database, "server_name": c.ServerName,
		"application_name": c.ApplicationName,
	} {
		if strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("postgres: %s contains a NUL byte", name)
		}
	}
	for key, value := range c.RuntimeParams {
		if key == "" {
			return errors.New("postgres: runtime parameter name is empty")
		}
		if strings.ContainsRune(key, '\x00') || strings.ContainsRune(value, '\x00') {
			return errors.New("postgres: runtime parameter contains a NUL byte")
		}
	}
	return nil
}

func (c Config) address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(int(c.Port)))
}
