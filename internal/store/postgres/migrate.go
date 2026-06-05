package postgres

import (
	"context"
	"database/sql"
	"embed"

	"github.com/elythi0n/virta/internal/store/sqlcommon"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrate applies the Postgres migrations using the shared runner, rebinding the
// version-insert placeholder to Postgres' positional style.
func migrate(ctx context.Context, db *sql.DB) error {
	return sqlcommon.Migrate(ctx, db, migrationsFS, rebind)
}
