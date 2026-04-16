package migrations

import (
	"io/fs"
	"strings"
	"testing"
)

func TestFSContainsMigrations(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatalf("fs.ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one migration file, got 0")
	}

	var upCount int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			upCount++
		}
	}
	if upCount == 0 {
		t.Fatalf("expected at least one *.up.sql file among %d entries", len(entries))
	}
}
