# Security policy

## Supported versions

Before `v1.0.0`, only the latest release candidate is supported. After the stable release, security fixes target the latest `v1.x` version.

## Reporting

Report suspected vulnerabilities through GitHub private vulnerability reporting. Do not open a public issue containing credentials, certificate material, PostgreSQL cancel keys, private infrastructure, or production data.

Include the affected version or commit, Go/PostgreSQL versions, operating system, minimal reproduction, realistic impact, and whether secrets or production data were involved.

## Security boundaries

The driver:

- separates SQL text and runtime values through the extended query protocol;
- rejects malformed message lengths and enforces a configurable message-size limit;
- defaults to TLS certificate and hostname verification;
- verifies SCRAM server signatures in constant time;
- validates SCRAM nonces and iteration counts;
- exposes PostgreSQL errors structurally rather than parsing localized text;
- cancels timed-out queries and drains the connection back to `ReadyForQuery` before reuse;
- marks desynchronized or failed network connections unusable.

The application remains responsible for:

- using parameter placeholders instead of SQL concatenation;
- selecting safe pool limits and context deadlines;
- protecting DSNs and database credentials;
- managing trusted CA roots;
- reviewing database privileges and PostgreSQL server configuration;
- not exposing raw PostgreSQL errors directly to untrusted clients;
- not logging query arguments that may contain credentials or tokens.

## Known v1 boundaries

- `SCRAM-SHA-256-PLUS` channel binding is not in v1 scope. Production deployments should still use `verify-full` TLS.
- Cancel requests use protocol 3.0's four-byte key and a separate TCP connection, matching the negotiated protocol. Treat network access to PostgreSQL as privileged.
- Cleartext password authentication is rejected on plaintext connections unless `AllowInsecureAuthentication` is explicitly enabled.
- MD5 authentication exists for compatibility; SCRAM-SHA-256 is preferred.
- PostgreSQL passwords with uncommon SASLprep-sensitive Unicode sequences are not part of the v1 compatibility promise. Use generated ASCII service credentials.
