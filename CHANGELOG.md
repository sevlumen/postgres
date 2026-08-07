# Changelog

## [1.1.1] - 2026-08-07

### Fixed

- restore standard `database/sql` pointer-parameter behavior through the custom named-value checker;
- dereference non-nil pointers to supported positional parameter values;
- encode nil pointers as PostgreSQL `NULL`;
- honor `driver.Valuer` implementations such as `sql.NullString` while preserving wrapped conversion errors;
- add PostgreSQL 14–18 integration coverage for pointer and Valuer parameters.

## [1.1.0] - 2026-08-07

### Added

- pinned `Session` acquisition over `database/sql` for advisory locks and other session-scoped operations;
- explicit `Session.Discard` for backends that must never return to the pool;
- trusted multi-statement `ExecScriptContext` through PostgreSQL's simple-query protocol;
- script support for dollar-quoted procedural bodies and deterministic command-tag reporting;
- PostgreSQL 14–18 integration coverage for session pinning, script transactions, cancellation, protocol recovery, and pool reuse.

### Security

- keep application values on parameterized `ExecContext`, `QueryContext`, and `QueryRowContext` paths;
- document that script input is trusted developer-authored SQL and must never contain interpolated request data;
- discard canceled or protocol-uncertain script backends before they can be reused.

## [1.0.0] - 2026-08-07

### Added

- dependency-free PostgreSQL protocol 3.0 client;
- `database/sql` connector and `sevlumen-postgres` stdlib registration;
- TLS disable/require/verify-ca/verify-full modes;
- SCRAM-SHA-256 with RFC 7677 coverage, MD5 compatibility, and protected cleartext authentication;
- extended-query parameter binding;
- query, exec, statement, transaction, pool, cancellation, and safe early-row-close behavior;
- structured PostgreSQL errors and SQLSTATE helpers;
- `ErrOperationOutcomeUnknown` for ambiguous connection failures after an operation may have reached PostgreSQL;
- text codecs required by Identity workloads;
- Identity refresh-token rotation, transaction, failure-injection, malformed-frame fuzz, race, and PostgreSQL integration tests;
- release gates for Go 1.23/current, Linux, macOS, Windows, and PostgreSQL 14 through 18.

### Security

- require successful SCRAM server-final signature verification before authentication completes;
- reject cleartext and MD5 password authentication on unencrypted transports by default;
- reject reserved runtime startup parameters that could override credentials or stable codec settings;
- keep DSN credentials, passwords, query arguments, and cancellation secrets out of errors and logs;
- pin GitHub Actions to reviewed immutable revisions and run `govulncheck` as a required gate.

### Fixed

- apply `ConnectTimeout` to TCP dial, SSL negotiation, TLS handshake, and startup;
- preserve a caller-supplied TLS minimum stricter than TLS 1.2;
- prevent `database/sql` from automatically retrying operations whose outcome is unknown after a connection failure;
- prevent delayed `CancelRequest` packets from canceling a later query on a reused backend connection;
- drain `Rows.Close` through `ReadyForQuery` with a bounded timeout so `QueryRow` and transactions remain usable;
- preserve `context.Canceled` and `context.DeadlineExceeded` instead of masking them as bad-connection errors;
- preserve transaction context for commit while keeping rollback cleanup bounded;
- decode timestamps, timestamptz, numeric, bytea, JSON, and JSONB consistently across supported PostgreSQL versions.
