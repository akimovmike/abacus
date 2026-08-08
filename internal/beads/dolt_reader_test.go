package beads

import (
	"context"
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
	return &doltClient{r: &doltRunner{dir: "/x", run: run, mu: &sync.Mutex{}}, refreshMu: &sync.Mutex{}}
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
