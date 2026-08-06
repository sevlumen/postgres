# Support policy

The planned v1 line supports:

- Go 1.23 and newer under the Go 1 compatibility promise;
- PostgreSQL 14 through 18;
- Linux, macOS, and Windows;
- TCP connections;
- `database/sql` as the pooling and application-facing contract.

Supported reports include protocol correctness, TLS/authentication behavior, parameter encoding, result decoding, transaction behavior, cancellation, SQLSTATE mapping, connection reuse, concurrency, and documented compatibility regressions.

Out of scope for v1 are Unix sockets, protocol 3.2-only features, COPY, replication, public LISTEN/NOTIFY APIs, binary codecs, and undocumented PostgreSQL forks or proxies.
