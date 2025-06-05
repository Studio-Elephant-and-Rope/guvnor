# Database Migrations

Guvnor uses [golang-migrate](https://github.com/golang-migrate/migrate) for database schema management. This document covers migration strategy, usage, and best practices.

## Overview

The migration system provides:
- **Versioned schema changes** - Each migration has a unique version number
- **Up and down migrations** - Every change can be rolled back
- **Idempotent operations** - Safe to run migrations multiple times
- **Pre-flight checks** - Validates environment before applying changes
- **Atomic operations** - Changes are applied in transactions

## Architecture

```
migrations/
├── 001_initial_schema.up.sql    # Creates core tables
├── 001_initial_schema.down.sql  # Drops core tables
├── 002_add_schedules.up.sql     # Enhanced scheduling
└── 002_add_schedules.down.sql   # Removes scheduling enhancements
```

## Database Schema

### Core Tables

The initial migration creates these tables:

#### incidents
Core incident tracking with JSONB labels for flexibility.
- **Primary Key**: UUID
- **Indexes**: status, severity, team_id, created_at, labels (GIN)
- **Constraints**: resolved_at timestamp rules

#### events
Audit trail for all incident changes.
- **Foreign Key**: incident_id → incidents(id)
- **Indexes**: incident_id, type, actor, occurred_at
- **JSONB**: metadata field for flexible event data

#### signals
Incoming alerts from monitoring systems.
- **Indexes**: source, severity, received_at
- **JSONB**: labels, annotations, payload for OpenTelemetry data

#### schedules
On-call scheduling configuration.
- **Indexes**: team_id, active status
- **JSONB**: config field for schedule definitions

### Enhanced Tables (Migration 002)

#### escalation_policies
Define how incidents escalate through teams.
- **Foreign Key**: Referenced by incidents
- **JSONB**: policy_config for complex escalation rules

#### audit_logs
System-wide audit trail for compliance.
- **Indexes**: entity_type, entity_id, actor, occurred_at
- **JSONB**: changes and metadata for detailed tracking

## Usage

### Apply All Pending Migrations

```bash
# Apply all pending migrations
guvnor migrate up

# With custom config file
guvnor migrate up --config /path/to/config.yaml
```

### Rollback Last Migration

```bash
# Rollback one migration (with confirmation in production)
guvnor migrate down

# In production, you'll be prompted:
# ⚠️  You are about to rollback a migration in production. This may result in data loss.
# Type 'yes' to continue:
```

### Check Migration Status

```bash
# Show current migration status
guvnor migrate status

# Example output:
# Migration Status: Version 2
# Database Type: postgres
# Connection: ✅ OK
```

## Configuration

Migrations use the same configuration as the main application:

```yaml
storage:
  type: postgres
  dsn: "postgres://user:password@localhost:5432/guvnor?sslmode=disable"
```

### Environment Variables

Override configuration with environment variables:

```bash
export GUVNOR_STORAGE_TYPE=postgres
export GUVNOR_STORAGE_DSN="postgres://user:password@localhost:5432/guvnor"
guvnor migrate up
```

## Pre-flight Checks

Before applying migrations, the system validates:

1. **Database connectivity** - Can connect to the database
2. **Required permissions** - Can create tables and indexes
3. **Migration files** - All migration files are present and valid
4. **Database state** - Not in a "dirty" state from failed migrations

Example pre-flight check output:
```
Running pre-flight checks
✅ Database connection successful
✅ Migration files validated (2 migrations found)
✅ Pre-flight checks passed
```

## Best Practices

### Writing Migrations

1. **Always provide down migrations** - Every up migration must have a corresponding down migration
2. **Use idempotent operations** - Migrations should be safe to run multiple times
3. **Add constraints carefully** - Ensure data exists before adding NOT NULL constraints
4. **Index strategically** - Add indexes for expected query patterns

Example idempotent migration:
```sql
-- Good: Idempotent
CREATE TABLE IF NOT EXISTS new_table (...);
ALTER TABLE existing_table ADD COLUMN IF NOT EXISTS new_column TEXT;

-- Bad: Not idempotent
CREATE TABLE new_table (...);
ALTER TABLE existing_table ADD COLUMN new_column TEXT;
```

### Schema Changes

1. **Backwards compatibility** - Don't break existing application versions
2. **Data preservation** - Never lose data during migrations
3. **Performance impact** - Consider migration duration on large tables
4. **Rollback safety** - Ensure rollbacks don't lose critical data

### Production Deployments

1. **Backup first** - Always backup production databases
2. **Test migrations** - Run migrations on staging with production data
3. **Monitor performance** - Watch for long-running migrations
4. **Plan rollbacks** - Have a rollback plan ready

## Troubleshooting

### Dirty State

If a migration fails partway through, the database may be in a "dirty" state:

```
Migration Status: Version 1 (DIRTY - needs manual intervention)
⚠️  WARNING: Database is in dirty state.
   This usually means a migration failed partway through.
   Manual intervention may be required to fix the issue.
```

**Resolution:**
1. Identify what went wrong by checking database logs
2. Manually fix any partial changes
3. Use migrate CLI to force version and clear dirty state
4. Re-run migrations

### Connection Issues

**Error:** `database connection failed`

**Solutions:**
1. Check database is running: `pg_isready -h localhost -p 5432`
2. Verify credentials in configuration
3. Check network connectivity
4. Ensure database exists

### Permission Issues

**Error:** `failed to create migrator: permission denied`

**Solutions:**
1. Grant necessary permissions: `GRANT CREATE ON DATABASE guvnor TO guvnor_user;`
2. Check user has table creation rights
3. Verify user can create indexes

### Migration File Issues

**Error:** `no migration files found`

**Solutions:**
1. Check migrations directory exists
2. Verify migration files follow naming convention
3. Ensure both .up.sql and .down.sql files exist

## Development Workflow

### Creating New Migrations

1. **Generate migration files:**
   ```bash
   # Create new migration files
   touch migrations/003_add_feature.up.sql
   touch migrations/003_add_feature.down.sql
   ```

2. **Write up migration:**
   ```sql
   -- 003_add_feature.up.sql
   CREATE TABLE new_feature (
       id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
       name VARCHAR(255) NOT NULL,
       created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
   );

   CREATE INDEX idx_new_feature_name ON new_feature(name);
   ```

3. **Write down migration:**
   ```sql
   -- 003_add_feature.down.sql
   DROP TABLE IF EXISTS new_feature;
   ```

4. **Test locally:**
   ```bash
   guvnor migrate up    # Apply migration
   guvnor migrate down  # Test rollback
   guvnor migrate up    # Reapply
   ```

### Testing Migrations

Run the migration test suite:

```bash
# Run all migration tests (requires Docker)
go test -v ./internal/cmd -run TestMigration

# Run specific test
go test -v ./internal/cmd -run TestMigrationSystem

# Skip integration tests
go test -short ./internal/cmd
```

### Schema Validation

After migrations, validate the schema matches expectations:

```sql
-- Check table exists
SELECT tablename FROM pg_tables WHERE tablename = 'incidents';

-- Check indexes exist
SELECT indexname FROM pg_indexes WHERE tablename = 'incidents';

-- Check constraints
SELECT conname FROM pg_constraint WHERE conrelid = 'incidents'::regclass;
```

## Monitoring

### Metrics to Track

1. **Migration duration** - How long migrations take
2. **Success rate** - Percentage of successful migrations
3. **Database size growth** - Impact of schema changes
4. **Query performance** - Before/after migration performance

### Logging

Migrations produce structured logs:

```json
{
  "level": "INFO",
  "msg": "Starting database migration (up)",
  "current_version": 1,
  "migrations_found": 2
}
```

Set log level to DEBUG for detailed migration progress:

```bash
export GUVNOR_LOG_LEVEL=debug
guvnor migrate up
```

## Security Considerations

1. **Principle of least privilege** - Migration user should have minimal required permissions
2. **Audit migrations** - All schema changes should be reviewed
3. **Secure connections** - Always use SSL in production
4. **Backup encryption** - Encrypt database backups

## References

- [golang-migrate documentation](https://github.com/golang-migrate/migrate)
- [PostgreSQL documentation](https://www.postgresql.org/docs/)
- [Database migration best practices](https://www.prisma.io/dataguide/types/relational/migration-strategies)
