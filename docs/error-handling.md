# Error handling

## PostgreSQL errors

Database errors are returned as `*postgres.Error` with SQLSTATE and structured fields such as schema, table, column, and constraint. Use helpers such as `postgres.IsUniqueViolation` for expected database conditions.

## Connection failures before an operation

When a pooled connection is already known to be unusable before an operation starts, the driver returns `driver.ErrBadConn`. This allows `database/sql` to discard the connection and safely retry on another connection.

## Unknown operation outcomes

When the connection fails after an operation may have reached PostgreSQL, the driver returns `postgres.ErrOperationOutcomeUnknown` rather than `driver.ErrBadConn`.

```go
_, err := db.ExecContext(ctx, statement, args...)
if errors.Is(err, postgres.ErrOperationOutcomeUnknown) {
    // The statement may or may not have completed. Do not retry blindly.
    // Resolve the outcome through an idempotency key, unique constraint,
    // transaction record, or an application-specific status lookup.
}
```

This distinction prevents `database/sql` from automatically replaying a potentially non-idempotent write.

Identity operations should use transaction boundaries and stable identifiers. Refresh-token rotation should additionally use a unique token hash or rotation identifier so the application can determine whether a replacement token was committed after an ambiguous network failure.
