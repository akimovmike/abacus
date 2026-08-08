package beads

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newStubClient(t *testing.T, responses map[string]string) *doltClient {
	t.Helper()
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		q := args[2] // ["sql","-q",<sql>,"-r","json"]
		// Pick the longest (most specific) matching substring: a detail query
		// also contains "FROM issues", so a bare skeleton stub for "FROM
		// issues" must not shadow a more specific "description,design" or
		// "close_reason" stub registered in the same responses map.
		matched := ""
		var resp []byte
		for substr, r := range responses {
			if strings.Contains(q, substr) && len(substr) > len(matched) {
				matched = substr
				resp = []byte(r)
			}
		}
		if matched != "" {
			return resp, nil
		}
		return []byte(`{}`), nil
	}
	return &doltClient{r: &doltRunner{dir: "/x", run: run, sem: make(chan struct{}, 1)}, refreshMu: &sync.Mutex{}}
}

func TestDoltSkeletonAssembles(t *testing.T) {
	c := newStubClient(t, map[string]string{
		"FROM issues":       `{"rows":[{"id":"ab-1","title":"T","status":"open","issue_type":"task","priority":2,"assignee":null,"created_by":"Al","created_at":"2026-01-20 18:53:52","updated_at":"2026-01-20 18:53:52","closed_at":null}]}`,
		"FROM labels":       `{"rows":[{"issue_id":"ab-1","label":"ui"}]}`,
		"FROM dependencies": `{"rows":[{"issue_id":"ab-1","type":"blocks","depends_on_issue_id":"ab-2"}]}`,
	})
	got, err := c.skeleton(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "ab-1" || got[0].Title != "T" {
		t.Fatalf("issues=%v", got)
	}
	if got[0].DetailLoaded {
		t.Error("skeleton must leave DetailLoaded=false")
	}
	if got[0].CreatedBy != "Al" {
		t.Errorf("created_by=%q, want Al", got[0].CreatedBy)
	}
	if len(got[0].Labels) != 1 || got[0].Labels[0] != "ui" {
		t.Errorf("labels=%v", got[0].Labels)
	}
	if len(got[0].Dependencies) != 1 || got[0].Dependencies[0].Type != "blocks" || got[0].Dependencies[0].TargetID != "ab-2" {
		t.Errorf("deps=%v", got[0].Dependencies)
	}
}

func TestDoltSkeletonAssemblesReverseDependents(t *testing.T) {
	c := newStubClient(t, map[string]string{
		"FROM issues": `{"rows":[
			{"id":"ab-1","title":"T1","status":"open","issue_type":"task","priority":2,"assignee":null,"created_by":"Al","created_at":"2026-01-20 18:53:52","updated_at":"2026-01-20 18:53:52","closed_at":null},
			{"id":"ab-2","title":"T2","status":"open","issue_type":"task","priority":1,"assignee":null,"created_by":"Al","created_at":"2026-01-20 18:53:53","updated_at":"2026-01-20 18:53:53","closed_at":null}
		]}`,
		"FROM dependencies": `{"rows":[{"issue_id":"ab-1","type":"blocks","depends_on_issue_id":"ab-2"},{"issue_id":"ab-1","type":"parent-child","depends_on_issue_id":"ab-2"}]}`,
	})
	got, err := c.skeleton(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]FullIssue, len(got))
	for _, iss := range got {
		byID[iss.ID] = iss
	}
	if len(byID["ab-1"].Dependencies) != 2 {
		t.Fatalf("ab-1 deps=%v", byID["ab-1"].Dependencies)
	}
	if len(byID["ab-2"].Dependents) != 2 {
		t.Fatalf("ab-2 dependents=%v", byID["ab-2"].Dependents)
	}
	if byID["ab-2"].Dependents[0].ID != "ab-1" {
		t.Errorf("dependent id=%q, want ab-1", byID["ab-2"].Dependents[0].ID)
	}
}

func TestDoltSkeletonRejectsBlankRequired(t *testing.T) {
	c := newStubClient(t, map[string]string{
		"FROM issues": `{"rows":[{"id":"","title":"","status":"","issue_type":"","priority":0}]}`,
	})
	if _, err := c.skeleton(context.Background(), ""); err == nil {
		t.Fatal("expected schema-mismatch error on blank required fields")
	}
}

func TestDoltSkeletonRejectsBlankRequiredPerRow(t *testing.T) {
	c := newStubClient(t, map[string]string{
		"FROM issues": `{"rows":[
			{"id":"ab-1","title":"T","status":"open","issue_type":"task","priority":2},
			{"id":"ab-2","title":"","status":"open","issue_type":"task","priority":2}
		]}`,
	})
	if _, err := c.skeleton(context.Background(), ""); err == nil {
		t.Fatal("expected error on second row missing title")
	}
}

// TestValidateSkeletonNotPreloadedRejectsNonNilComments is the prod runtime
// guard for ab-6irx.2 firing directly: if a future edit to skeletonWhere
// reintroduces a non-nil Comments placeholder, this must fail loudly at
// read time (see validateSkeletonNotPreloaded's doc comment) rather than
// silently defeating markExportedCommentsLoaded/transferCommentState again.
func TestValidateSkeletonNotPreloadedRejectsNonNilComments(t *testing.T) {
	if err := validateSkeletonNotPreloaded(FullIssue{ID: "ab-1", Comments: []Comment{}}); err == nil {
		t.Fatal("expected an error for a skeleton row with non-nil Comments")
	}
}

func TestValidateSkeletonNotPreloadedRejectsDetailLoaded(t *testing.T) {
	if err := validateSkeletonNotPreloaded(FullIssue{ID: "ab-1", DetailLoaded: true}); err == nil {
		t.Fatal("expected an error for a skeleton row with DetailLoaded=true")
	}
}

func TestValidateSkeletonNotPreloadedAllowsGenuineSkeletonRow(t *testing.T) {
	if err := validateSkeletonNotPreloaded(FullIssue{ID: "ab-1"}); err != nil {
		t.Fatalf("unexpected error for a genuine skeleton row: %v", err)
	}
}

func TestStrHelper(t *testing.T) {
	if got := str("hello"); got != "hello" {
		t.Errorf("str(string)=%q", got)
	}
	if got := str(nil); got != "" {
		t.Errorf("str(nil)=%q, want empty", got)
	}
	if got := str(42); got != "" {
		t.Errorf("str(non-string)=%q, want empty", got)
	}
}

func TestIntOfHelper(t *testing.T) {
	if got := intOf(float64(3)); got != 3 {
		t.Errorf("intOf(float64)=%d, want 3", got)
	}
	if got := intOf(5); got != 5 {
		t.Errorf("intOf(int)=%d, want 5", got)
	}
	if got := intOf(nil); got != 0 {
		t.Errorf("intOf(nil)=%d, want 0", got)
	}
	if got := intOf("not a number"); got != 0 {
		t.Errorf("intOf(string)=%d, want 0", got)
	}
}

func TestDoltLoadDetail(t *testing.T) {
	c := newStubClient(t, map[string]string{
		"description,design": `{"rows":[{"id":"ab-1","description":"D","design":"","notes":"N","acceptance_criteria":"","close_reason":"","external_ref":"X"}]}`,
		"FROM comments":      `{"rows":[{"id":"c1","issue_id":"ab-1","author":"Al","text":"hi","created_at":"2026-01-20 18:53:52"}]}`,
	})
	iss := &FullIssue{ID: "ab-1"}
	if err := c.loadDetail(context.Background(), "", iss); err != nil {
		t.Fatal(err)
	}
	if iss.Description != "D" || iss.Notes != "N" || iss.ExternalRef != "X" || !iss.DetailLoaded {
		t.Fatalf("detail not loaded: %+v", iss)
	}
	if len(iss.Comments) != 1 || iss.Comments[0].Text != "hi" {
		t.Fatalf("comments=%v", iss.Comments)
	}
}

func TestDoltCommentsEmpty(t *testing.T) {
	c := newStubClient(t, map[string]string{"FROM comments": `{}`})
	got, err := c.Comments(context.Background(), "ab-1")
	if err != nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestDoltExportAndShow(t *testing.T) {
	c := newStubClient(t, map[string]string{
		"HASHOF":             `{"rows":[{"h":"snap1"}]}`,
		"FROM issues":        `{"rows":[{"id":"ab-1","title":"T","status":"open","issue_type":"task","priority":1,"created_by":"Al","created_at":"2026-01-20 18:53:52","updated_at":"2026-01-20 18:53:52"}]}`,
		"description,design": `{"rows":[{"id":"ab-1","description":"D"}]}`,
		"FROM comments":      `{}`,
	})
	exp, err := c.Export(context.Background())
	if err != nil || len(exp) != 1 || exp[0].DetailLoaded {
		t.Fatalf("export=%v err=%v", exp, err)
	}
	shown, err := c.Show(context.Background(), []string{"ab-1"})
	if err != nil || len(shown) != 1 || !shown[0].DetailLoaded || shown[0].Description != "D" {
		t.Fatalf("show=%v err=%v", shown, err)
	}
}

// TestDoltShowIsTargetedNotFullScan is the regression guard for ab-6irx.3:
// Show must query issues/labels/dependencies scoped to the requested ids
// (an `id IN (...)` / `issue_id IN (...)` / `depends_on_issue_id IN (...)`
// clause) instead of skeleton's unconditional full-table scan. It captures
// every SQL statement the stub runner receives and asserts the
// skeleton-shaped issues query, the labels query, and the dependencies
// query are all scoped to the requested id -- never a bare unscoped scan
// (which is what skeleton's query looks like, and what Show used to funnel
// through). It also checks the functional contract: full skeleton fields +
// detail loaded for the requested id, plus a reverse Dependent populated
// from another issue (ab-2) that depends on it.
func TestDoltShowIsTargetedNotFullScan(t *testing.T) {
	var queries []string
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		q := args[2] // ["sql","-q",<sql>,"-r","json"]
		queries = append(queries, q)
		switch {
		case strings.Contains(q, "HASHOF"):
			return []byte(`{"rows":[{"h":"snap1"}]}`), nil
		case strings.Contains(q, "description,design"):
			return []byte(`{"rows":[{"id":"ab-1","description":"D"}]}`), nil
		case strings.Contains(q, "FROM comments"):
			return []byte(`{}`), nil
		case strings.Contains(q, "FROM issues"):
			return []byte(`{"rows":[{"id":"ab-1","title":"T","status":"open","issue_type":"task","priority":1,"created_by":"Al","created_at":"2026-01-20 18:53:52","updated_at":"2026-01-20 18:53:52"}]}`), nil
		case strings.Contains(q, "FROM labels"):
			return []byte(`{"rows":[{"issue_id":"ab-1","label":"ui"}]}`), nil
		case strings.Contains(q, "FROM dependencies"):
			return []byte(`{"rows":[{"issue_id":"ab-2","type":"blocks","depends_on_issue_id":"ab-1"}]}`), nil
		default:
			return []byte(`{}`), nil
		}
	}
	c := &doltClient{r: &doltRunner{dir: "/x", run: run, sem: make(chan struct{}, 1)}, refreshMu: &sync.Mutex{}}

	got, err := c.Show(context.Background(), []string{"ab-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "ab-1" || !got[0].DetailLoaded || got[0].Description != "D" {
		t.Fatalf("show=%v", got)
	}
	if len(got[0].Dependents) != 1 || got[0].Dependents[0].ID != "ab-2" {
		t.Fatalf("expected reverse dependent ab-2, got %v", got[0].Dependents)
	}

	for _, q := range queries {
		isSkeletonIssuesQuery := strings.Contains(q, skeletonCols) && strings.Contains(q, "FROM issues")
		if isSkeletonIssuesQuery && !strings.Contains(q, "id IN") {
			t.Errorf("issues skeleton query not scoped to id IN (...): %q", q)
		}
		if strings.Contains(q, "FROM labels") && !strings.Contains(q, "issue_id IN") {
			t.Errorf("labels query not scoped to issue_id IN (...): %q", q)
		}
		if strings.Contains(q, "FROM dependencies") {
			if !strings.Contains(q, "issue_id IN") || !strings.Contains(q, "depends_on_issue_id IN") {
				t.Errorf("dependencies query missing both-direction id scoping: %q", q)
			}
		}
	}
}

// TestDoltSkeletonLeavesCommentsNil guards against ab-6irx.2: a skeleton row
// must leave Comments nil (not a non-nil empty slice) so internal/ui's
// markExportedCommentsLoaded (Comments != nil => loaded) does not mark every
// dolt-backed issue's comments as already loaded before any real fetch runs
// — which previously caused transferCommentState's skip-guard to discard
// genuinely-loaded comments on every subsequent refresh.
func TestDoltSkeletonLeavesCommentsNil(t *testing.T) {
	c := newStubClient(t, map[string]string{
		"FROM issues": `{"rows":[{"id":"ab-1","title":"T","status":"open","issue_type":"task","priority":2,"created_by":"Al","created_at":"2026-01-20 18:53:52","updated_at":"2026-01-20 18:53:52"}]}`,
	})
	got, err := c.skeleton(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("issues=%v", got)
	}
	if got[0].Comments != nil {
		t.Fatalf("skeleton must leave Comments nil (not a placeholder empty slice), got %#v", got[0].Comments)
	}
}

// TestDoltLoadDetailReturnsNonNilCommentsForZeroRows confirms the flip side:
// once a real detail load runs, Comments becomes non-nil even when the issue
// has zero comments, so it reads as "loaded, confirmed empty" rather than
// "not yet loaded".
func TestDoltLoadDetailReturnsNonNilCommentsForZeroRows(t *testing.T) {
	c := newStubClient(t, map[string]string{
		"description,design": `{"rows":[{"id":"ab-1","description":"D"}]}`,
		"FROM comments":      `{}`,
	})
	iss := &FullIssue{ID: "ab-1"}
	if err := c.loadDetail(context.Background(), "", iss); err != nil {
		t.Fatal(err)
	}
	if iss.Comments == nil {
		t.Fatal("expected loadDetail to leave Comments non-nil even with zero comment rows")
	}
	if len(iss.Comments) != 0 {
		t.Fatalf("expected zero comments, got %v", iss.Comments)
	}
}

// TestNewDoltClient_MissingDirReturnsError is the dolt-reader conformance
// entry for NewClientForBackend's routing (backend.go): when no
// embeddeddolt/<database> directory exists under beadsDir, NewDoltClient
// must surface resolveDoltDir's error rather than returning a client that
// can never query anything. This runs before the dolt-version gate, so it
// needs no real dolt binary.
func TestNewDoltClient_MissingDirReturnsError(t *testing.T) {
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	if _, err := NewDoltClient(beadsDir, "missing", nil); err == nil {
		t.Fatal("expected error when embeddeddolt database directory does not exist")
	}
}

// TestNewDoltClient_ConstructsReadableClient exercises the real, unstubbed
// NewDoltClient constructor end to end: beadsDir/database resolution, the
// dolt-binary version gate, and a live `dolt sql` call against a genuine
// (schema-less) Dolt database. Unlike the rest of this file it does not
// inject a stub commandRunner, because NewDoltClient (unlike
// checkDoltVersion) always shells out via the unexported execDolt — so this
// is the only place that exercises that real wiring outside the
// integration-tagged fixtures (dolt_conformance_test.go). It skips when the
// dolt CLI is unavailable rather than failing the suite, mirroring how
// integration tests skip on a missing bd/br binary.
func TestNewDoltClient_ConstructsReadableClient(t *testing.T) {
	if !commandExists("dolt") {
		t.Skip("dolt binary not found, skipping NewDoltClient construction test")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir symlinks: %v", err)
	}
	beadsDir := filepath.Join(dir, ".beads")
	dbDir := filepath.Join(beadsDir, "embeddeddolt", "testdb")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("create embeddeddolt dir: %v", err)
	}
	initCmd := exec.Command("dolt", "init")
	initCmd.Dir = dbDir
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("dolt init: %v\n%s", err, out)
	}

	client, err := NewDoltClient(beadsDir, "testdb", nil)
	if err != nil {
		t.Fatalf("NewDoltClient: %v", err)
	}
	if _, ok := client.(*doltClient); !ok {
		t.Fatalf("expected *doltClient, got %T", client)
	}
	// A freshly-`dolt init`'d repo has no `issues` table yet, so Export must
	// surface that as an error instead of panicking or hanging — confirming
	// the client is really talking to this dolt database via `dolt sql`.
	if _, err := client.Export(context.Background()); err == nil {
		t.Fatal("expected Export to fail against a database with no issues table")
	}
}
