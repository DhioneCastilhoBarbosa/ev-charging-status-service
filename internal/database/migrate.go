package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

// Migrations já existentes em bancos que foram migrados manualmente antes do auto-migrate.
var legacyBaseline = []string{
	"001_init.sql",
	"002_webhook_events_payload_text.sql",
	"003_third_party_credentials_unique_user.sql",
	"004_drop_webhooks.sql",
}

// RunMigrations aplica arquivos *.sql de dir em ordem alfabética.
// Cada arquivo é registrado em schema_migrations (roda só uma vez).
// dir típico: "migrations" (copiado no Docker para /app/migrations).
func RunMigrations(db *sqlx.DB, dir string) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	if err := baselineExistingDatabase(db); err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("[migrate] directory %q not found — skip auto-migrate (aplique SQL manualmente se necessário)", dir)
			return nil
		}
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}
		if strings.EqualFold(name, "clear_data.sql") {
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)

	for _, name := range files {
		var exists bool
		if err := db.Get(&exists, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)`, name); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if exists {
			continue
		}

		path := filepath.Join(dir, name)
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		sqlText := strings.TrimSpace(string(body))
		if sqlText == "" {
			log.Printf("[migrate] skip empty %s", name)
			continue
		}

		tx, err := db.Beginx()
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err := tx.Exec(sqlText); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (filename) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
		log.Printf("[migrate] applied %s", name)
	}
	return nil
}

// baselineExistingDatabase evita reaplicar 001–004 em bancos já criados à mão.
func baselineExistingDatabase(db *sqlx.DB) error {
	var usersExists bool
	if err := db.Get(&usersExists, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'users'
		)
	`); err != nil {
		return fmt.Errorf("check users table: %w", err)
	}
	if !usersExists {
		return nil
	}

	var n int
	if err := db.Get(&n, `SELECT COUNT(*) FROM schema_migrations`); err != nil {
		return fmt.Errorf("count schema_migrations: %w", err)
	}
	if n > 0 {
		return nil
	}

	for _, name := range legacyBaseline {
		if _, err := db.Exec(`INSERT INTO schema_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING`, name); err != nil {
			return fmt.Errorf("baseline insert %s: %w", name, err)
		}
	}
	log.Printf("[migrate] baseline: marked %v as already applied (existing database)", legacyBaseline)
	return nil
}
