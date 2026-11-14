package migrations

import (
	"io/fs"
	"strings"
	"testing"
)

func TestLatestVersion(t *testing.T) {
	latest, err := LatestVersion()
	if err != nil {
		t.Fatalf("LatestVersion returned error: %v", err)
	}
	if latest != 6 {
		t.Errorf("LatestVersion = %d, want 6", latest)
	}
}

func TestEveryUpMigrationHasDownMigration(t *testing.T) {
	upFiles, err := fs.Glob(FS, "*.up.sql")
	if err != nil {
		t.Fatalf("glob up migrations: %v", err)
	}
	if len(upFiles) == 0 {
		t.Fatal("no up migrations found")
	}

	for _, upFile := range upFiles {
		downFile := strings.TrimSuffix(upFile, ".up.sql") + ".down.sql"
		if _, err := fs.Stat(FS, downFile); err != nil {
			t.Errorf("migration %s is missing %s: %v", upFile, downFile, err)
		}
	}
}
