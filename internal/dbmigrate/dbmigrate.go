// Package dbmigrate applies ordered, versioned schema migrations to a SQLite
// database, tracking progress in PRAGMA user_version.
package dbmigrate

import (
	"context"
	"database/sql"
	"fmt"
)

// Migration is one schema step. Version must be > 0 and strictly increasing
// across the list given to Apply. Up runs inside a transaction that also
// bumps user_version, so a failed migration leaves the database untouched.
type Migration struct {
	Version int
	Name    string
	Up      func(tx *sql.Tx) error
}

// SQL returns an Up function that executes the given statements.
func SQL(stmts ...string) func(tx *sql.Tx) error {
	return func(tx *sql.Tx) error {
		for _, s := range stmts {
			if _, err := tx.Exec(s); err != nil {
				return err
			}
		}
		return nil
	}
}

// Apply runs every migration newer than the database's user_version, each in
// its own transaction. Replaying is a no-op. It fails if the database is newer
// than the latest known migration.
func Apply(ctx context.Context, db *sql.DB, migrations []Migration) error {
	prev := 0
	for _, m := range migrations {
		if m.Version <= prev {
			return fmt.Errorf("dbmigrate: migration %d (%s) is not strictly greater than previous version %d", m.Version, m.Name, prev)
		}
		if m.Up == nil {
			return fmt.Errorf("dbmigrate: migration %d (%s) has no Up", m.Version, m.Name)
		}
		prev = m.Version
	}
	// A single connection keeps PRAGMA and transactions coherent.
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	current, err := userVersion(ctx, conn)
	if err != nil {
		return err
	}
	if current > prev {
		return fmt.Errorf("dbmigrate: database schema version %d is newer than this build supports (%d); upgrade the application", current, prev)
	}
	for _, m := range migrations {
		if m.Version <= current {
			continue
		}
		if err := applyOne(ctx, conn, m); err != nil {
			return fmt.Errorf("dbmigrate: migration %d (%s): %w", m.Version, m.Name, err)
		}
	}
	return nil
}

func userVersion(ctx context.Context, conn *sql.Conn) (int, error) {
	var v int
	err := conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&v)
	return v, err
}

func applyOne(ctx context.Context, conn *sql.Conn, m Migration) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Re-check under the transaction in case another process migrated meanwhile.
	var v int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v >= m.Version {
		return nil
	}
	if err := m.Up(tx); err != nil {
		return err
	}
	// PRAGMA does not accept bound parameters; Version is an int.
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, m.Version)); err != nil {
		return err
	}
	return tx.Commit()
}
