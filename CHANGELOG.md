# Changelog

## [1.0.0] - 2026-08-07

### Added

- dependency-free PostgreSQL protocol 3.0 client;
- `database/sql` connector and `sevlumen-postgres` stdlib registration;
- TLS disable/require/verify-ca/verify-full modes;
- SCRAM-SHA-256 with RFC 7677 coverage, MD5 compatibility, and protected cleartext authentication;
- extended-query parameter binding;
- query, exec, statement, transaction, pool, cancellation, and safe early-row-close behavior;
- structured PostgreSQL errors and SQLSTATE helpers;
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

- prevent delayed `CancelRequest` packets from canceling a later query on a reused backend connection;
- drain `Rows.Close` through `ReadyForQuery` with a bounded timeout so `QueryRow` and transactions remain usable;
- preserve `context.Canceled` and `context.DeadlineExceeded` instead of masking them as bad-connection errors;
- preserve transaction context for commit while keeping rollback cleanup bounded;
- decode timestamps, timestamptz, numeric, bytea, JSON, and JSONB consistently across supported PostgreSQL versions.
