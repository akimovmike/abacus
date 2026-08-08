//go:build integration

package beads

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// TestDoltIntegration_FixtureGoldenFields closes the durability gap left by
// TestDoltExportConformance (dolt_conformance_test.go): that test asserts
// only that required fields are non-empty, and skips solely on the `bd`
// binary. NewDoltClient reads via `dolt sql` (dolt_exec.go execDolt) — a
// completely separate binary from the `bd`-embedded Dolt library used by
// `bd init`/`bd create` — so a machine with `bd` but no `dolt` on PATH would
// fail this suite instead of skipping cleanly.
//
// This test builds an isolated, throwaway bd Dolt repository in t.TempDir()
// (never the live repo .beads — see setupBackendTestDB), seeds three issues
// with KNOWN field values via the real bd CLI, and reads them back through
// NewDoltClient end to end to assert an exact golden count plus exact
// field-level values (id/title/status/priority/issue_type/description) on a
// known issue, across both the Export skeleton path and the Show detail
// lazy-load path.
func TestDoltIntegration_FixtureGoldenFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd binary not found, skipping Dolt fixture integration test")
	}
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt binary not found, skipping Dolt fixture integration test")
	}

	env := setupBackendTestDB(t, "bd")
	defer env.cleanup()
	if env.StoreKind != StoreKindDolt {
		t.Skipf("bd initialized a %s store, skipping Dolt fixture integration test", env.StoreKind)
	}

	ctx := context.Background()
	fixtures := []struct {
		title       string
		issueType   string
		priority    int
		description string
	}{
		{"Fixture Bug One", "bug", 1, "Known description for golden test"},
		{"Fixture Task Two", "task", 2, "Second known description"},
		{"Fixture Chore Three", "chore", 3, "Third known description"},
	}

	ids := make([]string, 0, len(fixtures))
	for _, f := range fixtures {
		cmd := exec.Command("bd", "create",
			"--title", f.title,
			"--type", f.issueType,
			"--priority", intStr(f.priority),
			"--description", f.description,
			"--silent",
		)
		cmd.Dir = env.WorkDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bd create %q failed: %v\nOutput: %s", f.title, err, out)
		}
		id := strings.TrimSpace(string(out))
		if id == "" {
			t.Fatalf("bd create %q returned empty id, output: %s", f.title, out)
		}
		ids = append(ids, id)
	}

	client := newClientForBackend(t, env)

	// Golden count: exactly the fixtures seeded, no more, no less. A live
	// .beads DB would make this assertion flaky as issues are created/closed
	// elsewhere; the isolated temp-dir fixture keeps it deterministic.
	issues, err := client.Export(ctx)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if len(issues) != len(fixtures) {
		t.Fatalf("golden count mismatch: got %d issues, want %d (%+v)", len(issues), len(fixtures), issues)
	}

	wantID := ids[0]
	want := fixtures[0]

	var skeletonMatch *FullIssue
	for i := range issues {
		if issues[i].ID == wantID {
			skeletonMatch = &issues[i]
			break
		}
	}
	if skeletonMatch == nil {
		t.Fatalf("expected issue %s in Export results, got ids: %v", wantID, issueIDs(issues))
	}
	if skeletonMatch.Title != want.title {
		t.Errorf("Export: issue %s title = %q, want %q", wantID, skeletonMatch.Title, want.title)
	}
	if skeletonMatch.Status != "open" {
		t.Errorf("Export: issue %s status = %q, want %q", wantID, skeletonMatch.Status, "open")
	}
	if skeletonMatch.Priority != want.priority {
		t.Errorf("Export: issue %s priority = %d, want %d", wantID, skeletonMatch.Priority, want.priority)
	}
	if skeletonMatch.IssueType != want.issueType {
		t.Errorf("Export: issue %s issue_type = %q, want %q", wantID, skeletonMatch.IssueType, want.issueType)
	}
	if skeletonMatch.DetailLoaded {
		t.Error("Export: skeleton issue must leave DetailLoaded=false")
	}

	shown, err := client.Show(ctx, []string{wantID})
	if err != nil {
		t.Fatalf("Show(%s) failed: %v", wantID, err)
	}
	if len(shown) != 1 {
		t.Fatalf("Show(%s) returned %d issues, want 1", wantID, len(shown))
	}
	detail := shown[0]
	if detail.ID != wantID {
		t.Errorf("Show: id = %q, want %q", detail.ID, wantID)
	}
	if detail.Title != want.title {
		t.Errorf("Show: title = %q, want %q", detail.Title, want.title)
	}
	if detail.Status != "open" {
		t.Errorf("Show: status = %q, want %q", detail.Status, "open")
	}
	if detail.Priority != want.priority {
		t.Errorf("Show: priority = %d, want %d", detail.Priority, want.priority)
	}
	if detail.IssueType != want.issueType {
		t.Errorf("Show: issue_type = %q, want %q", detail.IssueType, want.issueType)
	}
	if detail.Description != want.description {
		t.Errorf("Show: description = %q, want %q", detail.Description, want.description)
	}
	if !detail.DetailLoaded {
		t.Error("Show: expected DetailLoaded=true")
	}
}

// issueIDs projects a slice of FullIssue down to their ids, for diagnostics.
func issueIDs(issues []FullIssue) []string {
	ids := make([]string, len(issues))
	for i, iss := range issues {
		ids[i] = iss.ID
	}
	return ids
}
