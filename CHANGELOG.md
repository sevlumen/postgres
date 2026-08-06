# Changelog

## [1.0.0-rc.1] - 2026-08-06

### Added

- dependency-free PostgreSQL protocol 3.0 client;
- `database/sql` connector and `sevlumen-postgres` stdlib registration;
- TLS disable/require/verify-ca/verify-full modes;
- SCRAM-SHA-256 with RFC 7677 test vector, MD5, and protected cleartext authentication;
- extended query parameter binding;
- query, exec, statement, transaction, pool, and cancellation support;
- structured PostgreSQL errors and SQLSTATE helpers;
- text codecs required by Identity workloads;
- malformed-frame fuzzing, race tests, PostgreSQL integration tests, and release gates.

### Release blockers

- PostgreSQL 14–18 CI matrix must pass on the published branch;
- cancellation and pool-reuse integration tests must pass repeatedly;
- public API and security review must be completed;
- release provenance and immutable action pins must be verified before the stable tag.
