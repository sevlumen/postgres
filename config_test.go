package postgres

import (
	"testing"
	"time"
)

func TestParseConfig(t *testing.T) {
	config, err := ParseConfig("postgres://user:p%40ss@db.example:5544/identity?sslmode=require&connect_timeout=3&application_name=identity")
	if err != nil {
		t.Fatal(err)
	}
	if config.Host != "db.example" || config.Port != 5544 {
		t.Fatalf("unexpected address: %#v", config)
	}
	if config.User != "user" || config.Password != "p@ss" || config.Database != "identity" {
		t.Fatalf("unexpected credentials/database: %#v", config)
	}
	if config.TLSMode != TLSRequire || config.ConnectTimeout != 3*time.Second || config.ApplicationName != "identity" {
		t.Fatalf("unexpected options: %#v", config)
	}
}

func TestConfigDefaultsAndValidation(t *testing.T) {
	config := Config{User: "identity"}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	if config.Database != "identity" || config.Port != 5432 || config.TLSMode != TLSVerifyFull {
		t.Fatalf("unexpected defaults: %#v", config)
	}
	invalid := Config{User: "identity", TLSMode: "prefer"}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected unsupported sslmode error")
	}
}

func TestConfigRejectsReservedRuntimeParameters(t *testing.T) {
	for _, name := range []string{"user", "database", "password", "client_encoding", "DateStyle", "bytea_output", "application_name"} {
		config := Config{User: "identity", RuntimeParams: map[string]string{name: "unexpected"}}
		if err := config.Validate(); err == nil {
			t.Fatalf("expected reserved runtime parameter %q to be rejected", name)
		}
	}
}
