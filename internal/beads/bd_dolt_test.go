package beads

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBdDoltClient_Export_MapsListJSON(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fakebd.sh")
	listJSON := `[{"id":"ab-1","title":"One","status":"open","priority":1,"issue_type":"task","labels":["a"],"dependencies":[{"id":"ab-2","dependency_type":"blocks"}]}]`
	writeTestScript(t, script, `#!/bin/sh
if [ "$1" = "-C" ] && [ "$3" = "list" ] && [ "$4" = "--json" ]; then
  echo '`+listJSON+`'
elif [ "$1" = "-C" ] && [ "$3" = "count" ] && [ "$4" = "--json" ]; then
  echo '{"count":1,"schema_version":1}'
fi
exit 0
`)

	client := NewBdDoltClient(dir, WithBdBinaryPath(script))
	issues, err := client.Export(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	iss := issues[0]
	if iss.ID != "ab-1" || iss.Title != "One" {
		t.Errorf("issue = %+v", iss)
	}
	if len(iss.Labels) != 1 || iss.Labels[0] != "a" {
		t.Errorf("labels = %v", iss.Labels)
	}
	if len(iss.Dependencies) != 1 || iss.Dependencies[0].TargetID != "ab-2" || iss.Dependencies[0].Type != "blocks" {
		t.Errorf("dependencies = %+v", iss.Dependencies)
	}
}

func TestBdDoltClient_Export_FiltersTombstones(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fakebd.sh")
	listJSON := `[{"id":"ab-1","title":"One","status":"open","priority":1,"issue_type":"task"},{"id":"ab-2","title":"Two","status":"tombstone","priority":1,"issue_type":"task"}]`
	writeTestScript(t, script, `#!/bin/sh
if [ "$1" = "-C" ] && [ "$3" = "list" ] && [ "$4" = "--json" ]; then
  echo '`+listJSON+`'
elif [ "$1" = "-C" ] && [ "$3" = "count" ] && [ "$4" = "--json" ]; then
  echo '{"count":1,"schema_version":1}'
fi
exit 0
`)

	client := NewBdDoltClient(dir, WithBdBinaryPath(script))
	issues, err := client.Export(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue after tombstone filter, got %d", len(issues))
	}
	if issues[0].ID != "ab-1" {
		t.Errorf("expected ab-1, got %s", issues[0].ID)
	}
}

func TestBdDoltClient_Export_CrossCheckCountOnZero(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fakebd.sh")
	writeTestScript(t, script, `#!/bin/sh
if [ "$1" = "-C" ] && [ "$3" = "list" ] && [ "$4" = "--json" ]; then
  echo '[]'
elif [ "$1" = "-C" ] && [ "$3" = "count" ] && [ "$4" = "--json" ]; then
  echo '{"count":5,"schema_version":1}'
fi
exit 0
`)

	client := NewBdDoltClient(dir, WithBdBinaryPath(script))
	_, err := client.Export(context.Background())
	if err == nil {
		t.Fatal("expected error when list is empty but count is non-zero")
	}
	if !strings.Contains(err.Error(), "count mismatch") {
		t.Errorf("expected count mismatch error, got: %v", err)
	}
}

func TestBdDoltClient_Show_FetchesByIDs(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fakebd.sh")
	showJSON := `[{"id":"ab-1","title":"One","status":"open","priority":1,"issue_type":"task"}]`
	writeTestScript(t, script, `#!/bin/sh
if [ "$1" = "-C" ] && [ "$3" = "show" ]; then
  echo '`+showJSON+`'
fi
exit 0
`)

	client := NewBdDoltClient(dir, WithBdBinaryPath(script))
	issues, err := client.Show(context.Background(), []string{"ab-1"})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "ab-1" {
		t.Fatalf("expected ab-1, got %+v", issues)
	}
}

func TestBdDoltClient_List_ProjectsLiteIssues(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fakebd.sh")
	listJSON := `[{"id":"ab-1","title":"One","status":"open","priority":1,"issue_type":"task"},{"id":"ab-2","title":"Two","status":"open","priority":2,"issue_type":"task"}]`
	writeTestScript(t, script, `#!/bin/sh
if [ "$1" = "-C" ] && [ "$3" = "list" ] && [ "$4" = "--json" ]; then
  echo '`+listJSON+`'
fi
exit 0
`)

	client := NewBdDoltClient(dir, WithBdBinaryPath(script))
	issues, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 lite issues, got %d", len(issues))
	}
	if issues[0].ID != "ab-1" || issues[1].ID != "ab-2" {
		t.Errorf("issues = %+v", issues)
	}
}

func TestBdDoltClient_Comments_MapsComments(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fakebd.sh")
	showJSON := `{"id":"ab-1","title":"One","status":"open","priority":1,"issue_type":"task","comments":[{"id":"7","issue_id":"ab-1","author":"alice","text":"hello","created_at":"2025-12-01T00:00:00Z"}]}`
	writeTestScript(t, script, `#!/bin/sh
if [ "$1" = "-C" ] && [ "$3" = "show" ]; then
  echo '`+showJSON+`'
fi
exit 0
`)

	client := NewBdDoltClient(dir, WithBdBinaryPath(script))
	comments, err := client.Comments(context.Background(), "ab-1")
	if err != nil {
		t.Fatalf("Comments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].ID != "7" || comments[0].Author != "alice" || comments[0].Text != "hello" {
		t.Errorf("comment = %+v", comments[0])
	}
}

func TestBdDoltClient_PassesWorkDir(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "args.log")
	script := filepath.Join(dir, "fakebd.sh")
	writeTestScript(t, script, `#!/bin/sh
echo "$@" >> `+logFile+`
echo '[]'
exit 0
`)

	client := NewBdDoltClient(dir, WithBdBinaryPath(script))
	_, _ = client.List(context.Background())

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.HasPrefix(string(data), "-C "+dir+" list --json") {
		t.Errorf("expected -C workdir prefix, got: %q", string(data))
	}
}
