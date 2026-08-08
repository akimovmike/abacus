package beads

import (
	"context"
	"testing"
	"time"
)

func TestDoltDeltaUpsertAndTombstone(t *testing.T) {
	prev := []FullIssue{
		{ID: "ab-1", Title: "old", Status: "open", IssueType: "task", UpdatedAt: "2026-01-20T10:00:00Z"},
		{ID: "ab-2", Title: "keep", Status: "open", IssueType: "task", UpdatedAt: "2026-01-20T09:00:00Z"},
	}
	c := newStubClient(t, map[string]string{
		"HASHOF": `{"rows":[{"h":"snap2"}]}`,
		// delta: ab-1 changed, ab-3 new, ab-2 tombstoned
		"updated_at >=": `{"rows":[
			{"id":"ab-1","title":"new","status":"open","issue_type":"task","priority":1,"created_at":"x","updated_at":"2026-01-20 11:00:00"},
			{"id":"ab-3","title":"fresh","status":"open","issue_type":"task","priority":1,"created_at":"x","updated_at":"2026-01-20 12:00:00"},
			{"id":"ab-2","title":"gone","status":"tombstone","issue_type":"task","priority":1,"created_at":"x","updated_at":"2026-01-20 12:00:00"}
		]}`,
		"FROM labels":       `{}`,
		"FROM dependencies": `{}`,
	})
	got, err := c.Delta(context.Background(), prev)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]FullIssue{}
	for _, i := range got {
		byID[i.ID] = i
	}
	if _, gone := byID["ab-2"]; gone {
		t.Error("ab-2 tombstoned should be dropped")
	}
	if byID["ab-1"].Title != "new" {
		t.Errorf("ab-1 not upserted: %q", byID["ab-1"].Title)
	}
	if _, ok := byID["ab-3"]; !ok {
		t.Error("ab-3 new should be present")
	}
}

func TestCapToNow(t *testing.T) {
	future := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	if got := capToNow(future); got == future {
		t.Errorf("capToNow(%q) = %q, want capped below the future value", future, got)
	}
	past := "2020-01-01T00:00:00Z"
	if got := capToNow(past); got != past {
		t.Errorf("capToNow(%q) = %q, want unchanged", past, got)
	}
}

func TestMaxUpdatedAtIgnoresTombstones(t *testing.T) {
	issues := []FullIssue{
		{ID: "ab-1", Status: "open", UpdatedAt: "2026-01-20T10:00:00Z"},
		{ID: "ab-2", Status: "tombstone", UpdatedAt: "2026-01-20T23:00:00Z"},
	}
	if got := maxUpdatedAt(issues); got != "2026-01-20T10:00:00Z" {
		t.Errorf("maxUpdatedAt=%q, want ab-1's timestamp (tombstone ignored)", got)
	}
}

func TestWmToDolt(t *testing.T) {
	got, err := wmToDolt("2026-01-20T10:00:00Z")
	if err != nil || got != "2026-01-20 10:00:00" {
		t.Fatalf("got %q err=%v", got, err)
	}
	if _, err := wmToDolt("not-a-time"); err == nil {
		t.Fatal("expected error for unparsable watermark")
	}
}

func TestDateTimeLiteral(t *testing.T) {
	lit, err := dateTimeLiteral("2026-01-20 10:00:00")
	if err != nil || lit != "'2026-01-20 10:00:00'" {
		t.Fatalf("got %q err=%v", lit, err)
	}
	if _, err := dateTimeLiteral("2026-01-20 10:00:00'; DROP TABLE issues; --"); err == nil {
		t.Fatal("expected reject for malformed/injection datetime")
	}
}

// TestDoltDeltaFallsBackToFullReadOnUnparsableWatermark exercises Delta's
// defensive fallback: a watermark that sorts lexicographically before "now"
// (so capToNow leaves it alone) but isn't valid RFC3339 (dolt's own space
// form, missing the "T"/"Z") must fail wmToDolt and fall back to a full
// skeletonWhere read rather than sending a broken SQL comparison.
func TestDoltDeltaFallsBackToFullReadOnUnparsableWatermark(t *testing.T) {
	prev := []FullIssue{
		{ID: "ab-1", Status: "open", UpdatedAt: "2026-01-20 10:00:00"},
	}
	c := newStubClient(t, map[string]string{
		"HASHOF":      `{"rows":[{"h":"snap4"}]}`,
		"FROM issues": `{"rows":[{"id":"ab-9","title":"full","status":"open","issue_type":"task","priority":1,"created_at":"x","updated_at":"2026-01-20 12:00:00"}]}`,
	})
	got, err := c.Delta(context.Background(), prev)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "ab-9" {
		t.Fatalf("expected fallback to full skeleton read, got %v", got)
	}
}

// TestDoltDeltaNoBaselineIsFullRead exercises Delta with an empty prev
// (no watermark): it must behave like a full skeleton read, not error.
func TestDoltDeltaNoBaselineIsFullRead(t *testing.T) {
	c := newStubClient(t, map[string]string{
		"HASHOF":      `{"rows":[{"h":"snap5"}]}`,
		"FROM issues": `{"rows":[{"id":"ab-1","title":"T","status":"open","issue_type":"task","priority":1,"created_at":"x","updated_at":"2026-01-20 12:00:00"}]}`,
	})
	got, err := c.Delta(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "ab-1" {
		t.Fatalf("expected full read with one issue, got %v", got)
	}
}

// TestAttachCommentFingerprintsStampsCountAndMaxCreatedAt is the
// beads-layer test for ab-6irx.6's fingerprint aggregate: a single GROUP BY
// query stamps each issue with "<count>|<max created_at>", scoped only to
// issue ids already present in byID (mirrors attachLabels/attachDeps).
func TestAttachCommentFingerprintsStampsCountAndMaxCreatedAt(t *testing.T) {
	c := newStubClient(t, map[string]string{
		"FROM comments": `{"rows":[
			{"issue_id":"ab-1","n":2,"maxc":"2026-01-20 11:00:00"},
			{"issue_id":"ab-unknown","n":5,"maxc":"2026-01-20 12:00:00"}
		]}`,
	})
	byID := map[string]*FullIssue{
		"ab-1": {ID: "ab-1"},
		"ab-2": {ID: "ab-2"}, // no comments -> stays at the zero value ""
	}
	if err := c.attachCommentFingerprints(context.Background(), "", byID); err != nil {
		t.Fatal(err)
	}
	want := "2|" + normalizeDoltTime("2026-01-20 11:00:00")
	if byID["ab-1"].CommentFingerprint != want {
		t.Errorf("ab-1 fingerprint=%q, want %q", byID["ab-1"].CommentFingerprint, want)
	}
	if byID["ab-2"].CommentFingerprint != "" {
		t.Errorf("ab-2 (zero comments) fingerprint=%q, want empty", byID["ab-2"].CommentFingerprint)
	}
	// ab-unknown isn't in byID; the aggregate row for it must be silently
	// skipped, not panic or leak into another issue's fingerprint.
}

// TestAttachCommentFingerprintsEmptyResultLeavesFingerprintsBlank confirms
// an issue set with no comments at all (aggregate query returns zero rows)
// leaves every CommentFingerprint at "" rather than erroring.
func TestAttachCommentFingerprintsEmptyResultLeavesFingerprintsBlank(t *testing.T) {
	c := newStubClient(t, map[string]string{"FROM comments": `{}`})
	byID := map[string]*FullIssue{"ab-1": {ID: "ab-1"}}
	if err := c.attachCommentFingerprints(context.Background(), "", byID); err != nil {
		t.Fatal(err)
	}
	if byID["ab-1"].CommentFingerprint != "" {
		t.Errorf("fingerprint=%q, want empty for an issue with no comments", byID["ab-1"].CommentFingerprint)
	}
}

// TestSkeletonStampsCommentFingerprint is the integration point between
// skeletonWhere and attachCommentFingerprints: a full skeleton read (the
// Export/Delta path) must come back with CommentFingerprint populated
// alongside labels/deps, not just from a direct attachCommentFingerprints
// call.
func TestSkeletonStampsCommentFingerprint(t *testing.T) {
	c := newStubClient(t, map[string]string{
		"FROM issues":   `{"rows":[{"id":"ab-1","title":"T","status":"open","issue_type":"task","priority":2,"created_by":"Al","created_at":"2026-01-20 18:53:52","updated_at":"2026-01-20 18:53:52"}]}`,
		"FROM comments": `{"rows":[{"issue_id":"ab-1","n":3,"maxc":"2026-01-20 20:00:00"}]}`,
	})
	got, err := c.skeleton(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	want := "3|" + normalizeDoltTime("2026-01-20 20:00:00")
	if len(got) != 1 || got[0].CommentFingerprint != want {
		t.Fatalf("issues=%+v, want CommentFingerprint=%q", got, want)
	}
}
