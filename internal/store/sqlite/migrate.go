package sqlite

import (
	"context"
	"database/sql"
	"embed"

	"github.com/elythi0n/virta/internal/store/sqlcommon"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrate applies the SQLite migrations using the shared runner. SQLite uses ? placeholders
// as written, so the rebind is the identity.
func migrate(ctx context.Context, db *sql.DB) error {
	return sqlcommon.Migrate(ctx, db, migrationsFS, func(q string) string { return q })
}
