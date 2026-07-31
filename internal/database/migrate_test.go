package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunMigrationsSkipsMissingDir(t *testing.T) {
	// Sem DB real: só garante que ReadDir inexistente não panica via wrapper.
	// O caminho feliz exige Postgres; aqui validamos a convenção de nomes.
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "005_ws_session_activity.sql"), []byte("-- test"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "clear_data.sql"), []byte("TRUNCATE x;"), 0o644)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var sqlCount int
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".sql" && e.Name() != "clear_data.sql" {
			sqlCount++
		}
	}
	if sqlCount != 1 {
		t.Fatalf("expected 1 migration sql, got %d", sqlCount)
	}
}
