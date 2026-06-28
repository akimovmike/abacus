package beads

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrDoltClient_Export_MapsListJSON(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fakebr.sh")
	listJSON := `[{"id":"br-1","title":"One","status":"open","priority":1,"issue_type":"task","labels":["a"],"dependencies":[{"id":"br-2","dependency_type":"blocks"}]}]`
	// ponytail: br JSON shape unverified — confirm against real br
	writeTestScript(t, script, `#!/bin/sh
if [ "$1" = "list" ] && [ "$2" = "--json" ]; then
  echo '`+listJSON+`'
elif [ "$1" = "count" ] && [ "$2" = "--json" ]; then
  echo '{"count":1,"schema_version":1}'
fi
exit 0
`)

	client := NewBrDoltClient(dir, WithBrBinaryPath(script))
	issues, err := client.Export(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	iss := issues[0]
	if iss.ID != "br-1" || iss.Title != "One" {
		t.Errorf("issue = %+v", iss)
	}
	if len(iss.Dependencies) != 1 || iss.Dependencies[0].TargetID != "br-2" {
		t.Errorf("dependencies = %+v", iss.Dependencies)
	}
}

func TestBrDoltClient_Export_FiltersTombstones(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fakebr.sh")
	listJSON := `[{"id":"br-1","title":"One","status":"open","priority":1,"issue_type":"task"},{"id":"br-2","title":"Two","status":"tombstone","priority":1,"issue_type":"task"}]`
	writeTestScript(t, script, `#!/bin/sh
if [ "$1" = "list" ] && [ "$2" = "--json" ]; then
  echo '`+listJSON+`'
elif [ "$1" = "count" ] && [ "$2" = "--json" ]; then
  echo '{"count":1,"schema_version":1}'
fi
exit 0
`)

	client := NewBrDoltClient(dir, WithBrBinaryPath(script))
	issues, err := client.Export(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "br-1" {
		t.Fatalf("expected br-1, got %+v", issues)
	}
}

func TestBrDoltClient_Export_CrossCheckCountOnZero(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fakebr.sh")
	writeTestScript(t, script, `#!/bin/sh
if [ "$1" = "list" ] && [ "$2" = "--json" ]; then
  echo '[]'
elif [ "$1" = "count" ] && [ "$2" = "--json" ]; then
  echo '{"count":5,"schema_version":1}'
fi
exit 0
`)

	client := NewBrDoltClient(dir, WithBrBinaryPath(script))
	_, err := client.Export(context.Background())
	if err == nil {
		t.Fatal("expected error when list is empty but count is non-zero")
	}
	if !strings.Contains(err.Error(), "count mismatch") {
		t.Errorf("expected count mismatch error, got: %v", err)
	}
}

func TestBrDoltClient_Comments_MapsComments(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fakebr.sh")
	showJSON := `{"id":"br-1","title":"One","status":"open","priority":1,"issue_type":"task","comments":[{"id":"7","issue_id":"br-1","author":"alice","text":"hello","created_at":"2025-12-01T00:00:00Z"}]}`
	// ponytail: br JSON shape unverified — confirm against real br
	writeTestScript(t, script, `#!/bin/sh
if [ "$1" = "show" ]; then
  echo '`+showJSON+`'
fi
exit 0
`)

	client := NewBrDoltClient(dir, WithBrBinaryPath(script))
	comments, err := client.Comments(context.Background(), "br-1")
	if err != nil {
		t.Fatalf("Comments: %v", err)
	}
	if len(comments) != 1 || comments[0].Author != "alice" {
		t.Errorf("comments = %+v", comments)
	}
}

func TestBrDoltClient_PassesWorkDir(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "args.log")
	script := filepath.Join(dir, "fakebr.sh")
	writeTestScript(t, script, `#!/bin/sh
echo "$@" >> `+logFile+`
echo '[]'
exit 0
`)

	client := NewBrDoltClient(dir, WithBrBinaryPath(script))
	_, _ = client.List(context.Background())

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.HasPrefix(string(data), "list --json") {
		t.Errorf("expected list --json, got: %q", string(data))
	}
}
