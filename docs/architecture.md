# Architecture

```text
Application / ORM
        |
        v
database/sql
        |
        v
postgres.Connector
        |
        +-- Config and TLS policy
        +-- PostgreSQL startup/authentication
        +-- Extended query protocol
        +-- Rows and text codecs
        +-- Transactions and SQLSTATE
        +-- CancelRequest and resynchronization
        |
        v
TCP/TLS -> PostgreSQL 14-18
```

## Dependency direction

The root package depends only on the Go standard library and internal protocol packages. `stdlib` imports the root package only to register a global driver name. No package depends on `pgx`, `libpq`, CGo, an ORM, or an Identity service.

## Protocol state

A connection is used by one operation at a time. `database/sql` supplies pooling and assigns an exclusive connection to a transaction. The driver additionally serializes wire access to protect direct connector consumers.

Every extended query sends `Parse`, `Bind`, portal `Describe`, `Execute`, and `Sync`. The connection is reusable only after a valid `ReadyForQuery`. On malformed frames, I/O loss, or protocol desynchronization, the connection is marked bad and discarded.

## Cancellation

The startup exchange stores `BackendKeyData`. When the operation context is canceled, a separate TCP connection sends `CancelRequest`. The original connection continues reading until `ReadyForQuery`. If resynchronization cannot complete before the cancellation grace period, the connection is closed and discarded.

## Pooling

Pooling is intentionally delegated to `database/sql`, whose behavior and operational controls are already part of the Go standard library. The driver implements `Connector`, `Pinger`, `SessionResetter`, `Validator`, `NamedValueChecker`, `ConnBeginTx`, and context-aware query/exec interfaces.
