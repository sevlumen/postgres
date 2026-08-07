# v1.0.0 release checklist

The source release is merge-ready after all code, compatibility, security, and documentation gates below pass. Tag publication remains a separate post-merge operation.

## Correctness

- [x] Unit, race, and vet suites pass on Go 1.23 and current stable Go.
- [x] PostgreSQL integration suite passes on PostgreSQL 14, 15, 16, 17, and 18.
- [x] SCRAM-SHA-256 RFC 7677 vector passes.
- [x] TLS require, verify-ca, verify-full, hostname mismatch, and unknown-CA tests pass.
- [x] Extended query parameters preserve SQL shape for injection-shaped values.
- [x] NULL, bool, integer, float, numeric, text, bytea, date, timestamp, timestamptz, UUID, JSON, and JSONB round trips pass.
- [x] Commit, rollback, read-only, and all supported isolation modes pass.
- [x] Duplicate-key, foreign-key, serialization, deadlock, and cancellation SQLSTATE tests pass.
- [x] Query cancellation returns promptly and the pool remains usable.
- [x] Early `Rows.Close` resynchronizes or discards the connection safely.
- [x] Concurrent pool and transaction stress tests pass repeatedly under `-race`.

## Security

- [x] Protocol decoder fuzz corpus completes without panic or excessive allocation.
- [x] Message-size limit and truncated-frame tests pass.
- [x] SCRAM nonce, iteration, proof, and server-signature failure tests pass.
- [x] Cleartext and MD5 authentication are rejected without TLS by default.
- [x] TLS minimum is 1.2 and verify-full is the default.
- [x] Reserved startup parameters cannot override credentials or stable codec settings.
- [x] DSNs, passwords, cancel keys, and query arguments are absent from errors and logs.
- [x] `govulncheck -test ./...` reports no reachable vulnerability.
- [x] GitHub Actions are pinned to reviewed immutable revisions.

## Compatibility and operations

- [x] Public API baseline is reviewed and frozen for v1.
- [x] PostgreSQL 14–18 support evidence is retained in required CI.
- [x] Windows, macOS, and Linux gates pass.
- [x] README, support, security, architecture, and upgrade policies are complete.
- [x] Identity application dogfood covers login, refresh rotation, logout, and concurrent refresh attempts.
- [x] Failure-injection tests cover server disconnect during query and transaction commit.

## Release

- [x] Changelog has a final `v1.0.0` section and no unresolved source blockers.
- [x] Source version is exactly `1.0.0`.
- [ ] Annotated `v1.0.0` tag points at the reviewed merge commit.
- [ ] Full release workflow passes from that exact tag.
- [ ] GitHub release notes include supported Go/PostgreSQL versions, known boundaries, checksums/provenance, and migration guidance.
