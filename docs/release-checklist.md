# v1.0.0 release checklist

A stable tag is blocked until every required item is checked with retained CI evidence.

## Correctness

- [ ] Unit, race, and vet suites pass on Go 1.23 and current stable Go.
- [ ] PostgreSQL integration suite passes on PostgreSQL 14, 15, 16, 17, and 18.
- [ ] SCRAM-SHA-256 RFC 7677 vector passes.
- [ ] TLS require, verify-ca, verify-full, hostname mismatch, and unknown-CA tests pass.
- [ ] Extended query parameters preserve SQL shape for injection-shaped values.
- [ ] NULL, bool, integer, float, numeric, text, bytea, date, timestamp, timestamptz, UUID, JSON, and JSONB round trips pass.
- [ ] Commit, rollback, read-only, and all supported isolation modes pass.
- [ ] Duplicate-key, foreign-key, serialization, deadlock, and cancellation SQLSTATE tests pass.
- [ ] Query cancellation returns promptly and the pool remains usable.
- [ ] Early `Rows.Close` resynchronizes or discards the connection safely.
- [ ] Concurrent pool and transaction stress tests pass repeatedly under `-race`.

## Security

- [ ] Protocol decoder fuzz corpus completes without panic or excessive allocation.
- [ ] Message-size limit and truncated-frame tests pass.
- [ ] SCRAM nonce, iteration, proof, and server-signature failure tests pass.
- [ ] Cleartext authentication is rejected without TLS by default.
- [ ] TLS minimum is 1.2 and verify-full is the default.
- [ ] DSNs, passwords, cancel keys, and query arguments are absent from errors and logs.
- [ ] `govulncheck ./...` reports no reachable vulnerability.
- [ ] GitHub Actions are pinned to reviewed immutable revisions before the stable tag.

## Compatibility and operations

- [ ] Public API baseline is reviewed and frozen.
- [ ] PostgreSQL 14–18 support evidence is attached to the release.
- [ ] Windows, macOS, and Linux compile gates pass.
- [ ] README, support, security, architecture, and upgrade policies are complete.
- [ ] Identity application dogfood covers login, refresh rotation, logout, and concurrent refresh attempts.
- [ ] At least one failure-injection exercise covers server disconnect during query and transaction commit.

## Release

- [ ] Changelog has a final `v1.0.0` section and no unresolved blockers.
- [ ] Annotated `v1.0.0` tag points at the reviewed commit.
- [ ] Full release workflow passes from that exact tag.
- [ ] GitHub release notes include supported Go/PostgreSQL versions, known boundaries, checksums/provenance, and migration guidance.
