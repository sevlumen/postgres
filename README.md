# Sevlumen PostgreSQL

A PostgreSQL-native `database/sql` driver written in Go without `pgx`, `libpq`, or C dependencies.

> Status: `v1.0.0-rc.1`. Do not tag `v1.0.0` until every gate in [docs/release-checklist.md](docs/release-checklist.md) passes on PostgreSQL 14 through 18.

## v1 scope

The stable v1 contract is intentionally focused on production service workloads such as Identity:

- PostgreSQL protocol 3.0 over TCP;
- TLS modes `disable`, `require`, `verify-ca`, and `verify-full`;
- SCRAM-SHA-256, MD5, and TLS-protected cleartext authentication;
- extended query protocol with positional parameters;
- `database/sql` connector and registered stdlib driver;
- query, exec, prepared-statement facade, transactions, and pooling through `database/sql`;
- context cancellation through PostgreSQL `CancelRequest`;
- structured PostgreSQL errors and SQLSTATE helpers;
- text codecs for bool, integers, floats, numeric, text, bytea, date, timestamp, timestamptz, UUID, JSON, and JSONB;
- PostgreSQL 14–18 compatibility matrix.

Explicitly outside v1:

- Unix-domain sockets;
- SCRAM-SHA-256-PLUS/channel binding;
- protocol 3.2 variable-length cancellation keys;
- COPY, LISTEN/NOTIFY public APIs, replication, and pipelining;
- binary parameter/result codecs;
- server-side prepared-statement caching.

Those exclusions do not block Identity workloads and prevent the first stable API from becoming unnecessarily broad.

## Install

```bash
go get github.com/sevlumen/postgres@v1.0.0-rc.1
```

## Recommended usage

```go
package main

import (
    "context"
    "log"
    "time"

    postgres "github.com/sevlumen/postgres"
)

func main() {
    db, err := postgres.Open(
        "postgres://identity:password@localhost:5432/identity?sslmode=verify-full",
    )
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    db.SetMaxOpenConns(20)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(30 * time.Minute)

    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

    if err := db.PingContext(ctx); err != nil {
        log.Fatal(err)
    }
}
```

## Registered `database/sql` driver

```go
import (
    "database/sql"
    _ "github.com/sevlumen/postgres/stdlib"
)

 db, err := sql.Open(
    "sevlumen-postgres",
    "postgres://identity:password@localhost:5432/identity?sslmode=require",
 )
```

Prefer `postgres.Open` or `sql.OpenDB(postgres.NewConnector(...))` because these paths validate configuration before returning the pool.

## Queries and parameters

```go
var id string
var email string

err := db.QueryRowContext(
    ctx,
    `SELECT id, email FROM users WHERE email = $1`,
    "admin@example.com",
).Scan(&id, &email)
```

Arguments are encoded separately from SQL through PostgreSQL's extended query protocol. Do not interpolate request values into SQL strings.

## Transactions

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil {
    return err
}
defer tx.Rollback()

if _, err := tx.ExecContext(ctx,
    `UPDATE refresh_tokens SET consumed_at = now()
     WHERE token_hash = $1 AND consumed_at IS NULL`, tokenHash,
); err != nil {
    return err
}

if _, err := tx.ExecContext(ctx,
    `INSERT INTO refresh_tokens(id, token_hash) VALUES($1, $2)`, id, replacementHash,
); err != nil {
    return err
}

return tx.Commit()
```

## SQLSTATE

```go
_, err := db.ExecContext(ctx, statement, email)
if postgres.IsUniqueViolation(err) {
    // duplicate email
}
```

`postgres.Error` exposes severity, SQLSTATE, detail, hint, schema, table, column, and constraint fields.

## TLS defaults

The default is `sslmode=verify-full`. Production services should keep that setting and provide a certificate chain trusted by the operating system or a custom `RootCAs` pool.

`sslmode=require` encrypts the transport but does not verify the server identity. `sslmode=disable` should be limited to local development and isolated CI.

## Validation

Local unit and race tests:

```bash
go vet ./...
go test -race ./...
```

PostgreSQL integration suite:

```bash
SEVLUMEN_POSTGRES_TEST_DSN='postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable' \
  go test -race ./integration -count=1
```

See [architecture](docs/architecture.md), [security boundaries](SECURITY.md), and the [release checklist](docs/release-checklist.md).
