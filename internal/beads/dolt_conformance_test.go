//go:build integration

package beads

import (
	"context"
	"os/exec"
	"slices"
	"testing"
)

// TestDoltExportConformance verifies that the bd Dolt reader correctly maps
// the JSON output of `bd list --json` to the FullIssue shape abacus expects.
// It skips when bd is unavailable or when bd initializes a non-Dolt store.
func TestDoltExportConformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	skipIfNoBackend(t, "bd")

	env := setupBackendTestDB(t, "bd")
	defer env.cleanup()
	if env.StoreKind != StoreKindDolt {
		t.Skipf("bd initialized a %s store, skipping Dolt conformance test", env.StoreKind)
	}

	ctx := context.Background()

	// Create a few issues via the bd CLI so the Dolt store has real data.
	created := []string{}
	for _, tc := range []struct {
		title    string
		typ      string
		priority int
		labels   []string
	}{
		{"Dolt Conformance Bug", "bug", 1, []string{"conformance", "dolt"}},
		{"Dolt Conformance Task", "task", 2, []string{"dolt"}},
	} {
		cmd := exec.Command("bd", "create", "--title", tc.title, "--type", tc.typ, "--priority", intStr(tc.priority))
		cmd.Dir = env.WorkDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bd create failed: %v\nOutput: %s", err, out)
		}
		id := extractCreatedID(string(out))
		if id == "" {
			t.Fatalf("could not extract ID from bd create output: %s", out)
		}
		for _, label := range tc.labels {
			cmd = exec.Command("bd", "label", "add", id, label)
			cmd.Dir = env.WorkDir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("bd label add failed: %v\nOutput: %s", err, out)
			}
		}
		created = append(created, id)
	}

	// Read the issues through the Dolt reader.
	client := NewBdDoltClient(env.WorkDir)
	issues, err := client.Export(ctx)
	if err != nil {
		t.Fatalf("Dolt Export failed: %v", err)
	}
	if len(issues) != len(created) {
		t.Fatalf("expected %d issues, got %d", len(created), len(issues))
	}

	requiredFields := []string{"id", "title", "status", "priority", "issue_type"}
	for i, iss := range issues {
		for _, field := range requiredFields {
			switch field {
			case "id":
				if iss.ID == "" {
					t.Errorf("issue[%d] has empty id", i)
				}
			case "title":
				if iss.Title == "" {
					t.Errorf("issue[%d] has empty title", i)
				}
			case "status":
				if iss.Status == "" {
					t.Errorf("issue[%d] has empty status", i)
				}
			case "issue_type":
				if iss.IssueType == "" {
					t.Errorf("issue[%d] has empty issue_type", i)
				}
			}
		}
		if iss.Priority < 1 {
			t.Errorf("issue[%d] has invalid priority: %d", i, iss.Priority)
		}
	}

	// Verify labels were mapped.
	found := false
	for _, iss := range issues {
		if slices.Contains(iss.Labels, "conformance") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected at least one issue to have the 'conformance' label, got labels: %+v", issues)
	}

	// Verify Show reads back a single issue with comments.
	if len(created) == 0 {
		t.Fatal("no created issues to show")
	}
	id := created[0]
	if err := client.AddComment(ctx, id, "dolt conformance comment"); err != nil {
		t.Fatalf("AddComment failed: %v", err)
	}
	shown, err := client.Show(ctx, []string{id})
	if err != nil {
		t.Fatalf("Dolt Show failed: %v", err)
	}
	if len(shown) != 1 {
		t.Fatalf("expected 1 issue from Show, got %d", len(shown))
	}
	comments, err := client.Comments(ctx, id)
	if err != nil {
		t.Fatalf("Dolt Comments failed: %v", err)
	}
	if len(comments) == 0 {
		t.Errorf("expected at least one comment for issue %s", id)
	}
}

func intStr(n int) string {
	return string(rune('0' + n))
}
