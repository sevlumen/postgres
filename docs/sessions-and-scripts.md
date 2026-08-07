# Pinned sessions and trusted scripts

## Pinned sessions

`postgres.Acquire` reserves one `database/sql` connection until the returned `Session` is closed or discarded. Use it only when PostgreSQL backend identity matters, including advisory locks, temporary objects, session variables, and migration execution.

```go
session, err := postgres.Acquire(ctx, db)
if err != nil {
    return err
}
defer session.Close()

if _, err := session.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
    return err
}
```

A session must not be used concurrently by multiple goroutines. Application request traffic should continue to use `*sql.DB`, which provides pooling and concurrency.

## Returning or discarding a backend

`Session.Close` returns a healthy backend to the pool. `Session.Discard` closes the network connection and marks it unusable before releasing it to `database/sql`.

Discard the session when cleanup cannot be verified. A migration runner should discard after a failed or false `pg_advisory_unlock` result because returning a backend that still owns a lock can deadlock future deployments.

## Trusted SQL scripts

`Session.ExecScriptContext` uses PostgreSQL's simple-query protocol and supports multiple statements, comments, strings, and dollar-quoted procedural bodies.

```go
if _, err := session.ExecContext(ctx, `BEGIN`); err != nil {
    return err
}
committed := false
defer func() {
    if !committed {
        _, _ = session.ExecContext(context.Background(), `ROLLBACK`)
    }
}()

if _, err := session.ExecScriptContext(ctx, migrationSQL); err != nil {
    return err
}
if _, err := session.ExecContext(ctx,
    `INSERT INTO migration_history(id, checksum) VALUES($1, $2)`,
    id,
    checksum,
); err != nil {
    return err
}
if _, err := session.ExecContext(ctx, `COMMIT`); err != nil {
    return err
}
committed = true
```

Scripts are a trusted developer-input boundary. The API intentionally accepts no arguments. Never concatenate request values, tenant values, credentials, file names, or other untrusted input into a script. Use parameterized `ExecContext`, `QueryContext`, or `QueryRowContext` for application data.

## Cancellation and uncertain outcomes

Cancellation sends PostgreSQL `CancelRequest`, drains protocol responses when possible, and discards the backend before it can affect later work. Network failure after a script may have reached PostgreSQL returns `postgres.ErrOperationOutcomeUnknown`. Resolve ambiguous outcomes through migration history or another idempotency record rather than retrying blindly.

## COPY boundary

`ExecScriptContext` rejects COPY protocol transitions. COPY remains outside the stable driver contract and must not be embedded in migration artifacts that rely on this API.
