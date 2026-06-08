package beads

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSQLiteDSN_EscapesReservedPathCharacters(t *testing.T) {
	t.Parallel()

	dsn := buildSQLiteDSN("/tmp/project#1/.beads/beads?.db")

	if !strings.HasPrefix(dsn, "file:/tmp/project%231/.beads/beads%3F.db?") {
		t.Fatalf("reserved path characters must be escaped in DSN: %q", dsn)
	}
	if strings.Contains(dsn, "/tmp/project#1/.beads/beads?.db?") {
		t.Fatalf("DSN must not include raw reserved characters in the path: %q", dsn)
	}
}

func TestBuildSQLiteDSN(t *testing.T) {
	t.Parallel()

	requiredPragmas := []string{
		"busy_timeout(30000)",
		"query_only(ON)",
		"foreign_keys(ON)",
	}
	assertParams := func(t *testing.T, dsn string) {
		t.Helper()
		queryStart := strings.Index(dsn, "?")
		if queryStart < 0 {
			t.Fatalf("DSN missing query string: %q", dsn)
		}
		values, err := url.ParseQuery(dsn[queryStart+1:])
		if err != nil {
			t.Fatalf("parse DSN query: %v", err)
		}
		if got := values.Get("mode"); got != "ro" {
			t.Errorf("mode = %q, want ro in %q", got, dsn)
		}
		for _, pragma := range requiredPragmas {
			if !containsString(values["_pragma"], pragma) {
				t.Errorf("DSN missing _pragma=%s: %q", pragma, dsn)
			}
		}
		for _, key := range []string{"_journal_mode", "_busy_timeout", "_foreign_keys"} {
			if _, ok := values[key]; ok {
				t.Errorf("DSN must not use standalone %s parameter: %q", key, dsn)
			}
		}
	}

	t.Run("unix absolute path", func(t *testing.T) {
		dsn := buildSQLiteDSN("/home/user/.beads/beads.db")
		if !strings.HasPrefix(dsn, "file:/home/user/.beads/beads.db?") {
			t.Errorf("unexpected DSN: %q", dsn)
		}
		assertParams(t, dsn)
	})

	t.Run("windows drive letter", func(t *testing.T) {
		dsn := buildSQLiteDSN("C:/Users/user/.beads/beads.db")
		if !strings.HasPrefix(dsn, "file:C:/Users/user/.beads/beads.db?") {
			t.Errorf("unexpected DSN: %q", dsn)
		}
		if strings.HasPrefix(dsn, "file://C") {
			t.Errorf("DSN must not use authority for drive letter: %q", dsn)
		}
		assertParams(t, dsn)
	})

	t.Run("relative path", func(t *testing.T) {
		dsn := buildSQLiteDSN("project/.beads/beads.db")
		if !strings.HasPrefix(dsn, "file:project/.beads/beads.db?") {
			t.Errorf("unexpected DSN: %q", dsn)
		}
		assertParams(t, dsn)
	})

	t.Run("UNC path", func(t *testing.T) {
		dsn := buildSQLiteDSN("//server/share/.beads/beads.db")
		if !strings.HasPrefix(dsn, "file:////server/share/.beads/beads.db?") {
			t.Errorf("UNC DSN must use four-slash form (file:////): %q", dsn)
		}
		if strings.HasPrefix(dsn, "file://server") {
			t.Errorf("DSN must not treat UNC host as authority: %q", dsn)
		}
		assertParams(t, dsn)
	})
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestSQLiteClients_ListWithReservedCharactersInDBPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		newClient func(string) Client
	}{
		{
			name: "br",
			newClient: func(dbPath string) Client {
				return NewBrSQLiteClient(dbPath)
			},
		},
		{
			name: "bd",
			newClient: func(dbPath string) Client {
				return NewBdSQLiteClient(dbPath)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dbPath := filepath.Join(t.TempDir(), "project#1", ".beads", "beads#1.db")
			createTestBrDB(t, dbPath)
			seedTestData(t, dbPath)

			issues, err := tt.newClient(dbPath).List(context.Background())
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(issues) != 3 {
				t.Fatalf("got %d issues, want 3", len(issues))
			}
		})
	}
}
