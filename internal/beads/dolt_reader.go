package beads

import (
	appErrors "abacus/internal/errors"
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"
)

// doltClient reads Beads data directly via `dolt sql` against an embedded
// Dolt database, delegating writes to an embedded CLI Writer.
type doltClient struct {
	r         *doltRunner
	refreshMu *sync.Mutex
	Writer
}

var _ Client = (*doltClient)(nil)

// NewDoltClient resolves the embedded Dolt database directory for database
// under beadsDir, verifies the dolt CLI meets MinDoltVersion, and returns a
// Client that reads via `dolt sql` and delegates writes to w.
func NewDoltClient(beadsDir, database string, w Writer) (Client, error) {
	dir, err := resolveDoltDir(beadsDir, database)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), versionCheckTimeout)
	defer cancel()
	if err := checkDoltVersion(ctx, execDolt); err != nil {
		return nil, err
	}
	return &doltClient{
		r:         &doltRunner{dir: dir, run: execDolt, sem: make(chan struct{}, 1)},
		refreshMu: &sync.Mutex{},
		Writer:    w,
	}, nil
}

// Export implements Reader: it takes a fresh snapshot and returns the
// skeleton (labels + dependencies, no heavy detail fields) of every
// non-tombstoned issue as of that snapshot. Concurrent callers are
// single-flighted via refreshMu so only one logical refresh runs at a time.
func (c *doltClient) Export(ctx context.Context) ([]FullIssue, error) {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	snap, err := c.r.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return c.skeleton(ctx, asOf(snap))
}

// List implements Reader: it returns the id of every issue Export would
// return, via a full Export (skeleton load), projected down to LiteIssue.
func (c *doltClient) List(ctx context.Context) ([]LiteIssue, error) {
	full, err := c.Export(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]LiteIssue, len(full))
	for i, iss := range full {
		out[i] = LiteIssue{ID: iss.ID}
	}
	return out, nil
}

// Show implements Reader: it takes a single snapshot, builds the skeleton
// once, then loads the heavy detail fields (and comments) for exactly the
// requested ids under that same snapshot, so every returned issue reflects
// one consistent point in history. Show does not take refreshMu: it snapshots
// independently of Export/List's single-flighted refresh.
func (c *doltClient) Show(ctx context.Context, ids []string) ([]FullIssue, error) {
	if len(ids) == 0 {
		return []FullIssue{}, nil
	}
	snap, err := c.r.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	asof := asOf(snap)
	all, err := c.skeleton(ctx, asof)
	if err != nil {
		return nil, err
	}
	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	var out []FullIssue
	for i := range all {
		if _, ok := want[all[i].ID]; !ok {
			continue
		}
		if err := c.loadDetail(ctx, asof, &all[i]); err != nil {
			return nil, err
		}
		out = append(out, all[i])
	}
	return out, nil
}

// skeletonCols lists the light issue columns read by skeleton (excludes the
// heavy text fields and external_ref, which the detail lazy-load in Task 7
// fetches separately).
const skeletonCols = "id,title,status,issue_type,priority,assignee,created_by,created_at,updated_at,closed_at"

// skeleton reads all non-tombstoned issues plus their labels and
// dependencies (both directions, all dependency types) as of asof (an
// " AS OF '<hash>'" clause from asOf, or "" for HEAD). Returned issues leave
// DetailLoaded=false; the heavy detail fields are populated by a later
// lazy-load. Every row is validated for required fields before assembly.
func (c *doltClient) skeleton(ctx context.Context, asof string) ([]FullIssue, error) {
	return c.skeletonWhere(ctx, asof, false, "")
}

// skeletonWhereClause builds the WHERE clause shared by skeleton and Delta:
// tombstoned issues are excluded unless includeTombstones is set (Delta
// needs to see tombstoned rows so its merge step can drop them), and
// extraWhere, when non-empty, is AND-ed onto the base clause (or used alone
// if there is no base clause).
func skeletonWhereClause(includeTombstones bool, extraWhere string) string {
	where := ""
	if !includeTombstones {
		where = "status != 'tombstone'"
	}
	if extraWhere == "" {
		return where
	}
	if where == "" {
		return extraWhere
	}
	return where + " AND " + extraWhere
}

// skeletonWhere is skeleton's shared query path, parameterized so Delta can
// reuse it: includeTombstones controls whether tombstoned issues are
// excluded, and extraWhere is an additional AND-ed condition (e.g. an
// updated_at watermark).
func (c *doltClient) skeletonWhere(
	ctx context.Context, asof string, includeTombstones bool, extraWhere string,
) ([]FullIssue, error) {
	where := skeletonWhereClause(includeTombstones, extraWhere)
	q := "SELECT " + skeletonCols + " FROM issues" + asof
	if where != "" {
		q += " WHERE " + where
	}
	issueRows, err := c.r.query(ctx, q+" ORDER BY created_at, id")
	if err != nil {
		return nil, err
	}

	byID := make(map[string]*FullIssue, len(issueRows))
	ordered := make([]*FullIssue, 0, len(issueRows))
	for _, row := range issueRows {
		iss := FullIssue{
			ID:           str(row["id"]),
			Title:        str(row["title"]),
			Status:       str(row["status"]),
			IssueType:    str(row["issue_type"]),
			Priority:     intOf(row["priority"]),
			Assignee:     str(row["assignee"]),
			CreatedBy:    str(row["created_by"]),
			CreatedAt:    normalizeDoltTime(str(row["created_at"])),
			UpdatedAt:    normalizeDoltTime(str(row["updated_at"])),
			ClosedAt:     normalizeDoltTime(str(row["closed_at"])),
			Labels:       []string{},
			Dependencies: []Dependency{},
			Dependents:   []Dependent{},
			// Comments is deliberately left nil (not []Comment{}): nil is the
			// "not yet loaded" signal internal/ui's markExportedCommentsLoaded
			// and applyLoadedComment rely on (Comments != nil => loaded),
			// matching bd_sqlite.go's loadBdComments convention. loadDetail
			// and Comments() below always return a non-nil slice (even for
			// zero rows, via make([]Comment, 0, ...)), so once real data is
			// fetched the signal flips correctly. Setting a non-nil empty
			// placeholder here made every dolt skeleton row look "already
			// loaded" to the UI layer: markExportedCommentsLoaded fired
			// immediately, and transferCommentState's skip-guard then
			// discarded genuinely-loaded comments on every refresh (ab-6irx.2).
		}
		if err := validateRequired(iss); err != nil {
			return nil, err
		}
		if err := validateSkeletonNotPreloaded(iss); err != nil {
			return nil, err
		}
		byID[iss.ID] = &iss
		ordered = append(ordered, &iss)
	}

	if err := c.attachLabels(ctx, asof, byID); err != nil {
		return nil, err
	}
	if err := c.attachDeps(ctx, asof, byID); err != nil {
		return nil, err
	}

	out := make([]FullIssue, len(ordered))
	for i, p := range ordered {
		out[i] = *p
	}
	return out, nil
}

// Delta computes an incremental refresh of prev: the watermark is the max
// non-tombstone UpdatedAt in prev, capped to the current time so a
// future-dated tombstone can never push it past live edits. With no
// watermark (prev empty, or every issue in it tombstoned) Delta behaves like
// a full skeleton read. Otherwise it takes one snapshot, fetches every issue
// (including tombstones) with updated_at >= the watermark, and merges the
// result into prev: tombstoned ids are dropped, everything else is upserted
// by id. Concurrent callers are single-flighted via refreshMu, same as
// Export.
func (c *doltClient) Delta(ctx context.Context, prev []FullIssue) ([]FullIssue, error) {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	snap, err := c.r.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	asof := asOf(snap)

	wm := capToNow(maxUpdatedAt(prev))
	if wm == "" {
		return c.skeletonWhere(ctx, asof, false, "")
	}
	doltWM, err := wmToDolt(wm)
	if err != nil {
		// Defensive: an unparsable watermark falls back to a full read
		// rather than risking a broken SQL comparison.
		return c.skeletonWhere(ctx, asof, false, "")
	}
	lit, err := dateTimeLiteral(doltWM)
	if err != nil {
		return nil, err
	}
	changed, err := c.skeletonWhere(ctx, asof, true, "updated_at >= "+lit)
	if err != nil {
		return nil, err
	}
	return mergeDelta(prev, changed), nil
}

// capToNow clamps wm to the current time so a future-dated tombstone can't
// push the watermark past live edits; both are RFC3339 UTC, so plain string
// comparison matches chronological order.
func capToNow(wm string) string {
	if now := time.Now().UTC().Format(time.RFC3339); wm > now {
		return now
	}
	return wm
}

// maxUpdatedAt returns the maximum UpdatedAt among issues' non-tombstone
// entries, or "" if there are none. UpdatedAt is normalized RFC3339, which
// sorts lexicographically, so plain string comparison gives chronological
// order.
func maxUpdatedAt(issues []FullIssue) string {
	max := ""
	for _, iss := range issues {
		if iss.Status != "tombstone" && iss.UpdatedAt > max {
			max = iss.UpdatedAt
		}
	}
	return max
}

// wmToDolt converts an RFC3339 watermark (as stored in FullIssue.UpdatedAt)
// back to dolt's "2006-01-02 15:04:05" space form for the SQL comparison.
func wmToDolt(wm string) (string, error) {
	t, err := time.Parse(time.RFC3339, wm)
	if err != nil {
		return "", fmt.Errorf("parse watermark %q: %w", wm, err)
	}
	return t.UTC().Format("2006-01-02 15:04:05"), nil
}

// doltDateTimeLiteralPattern matches exactly the "2006-01-02 15:04:05" space
// form wmToDolt produces. The shared sqlLiteral (dolt_exec.go) deliberately
// rejects spaces in its charset to reject injection via arbitrary strings;
// this value is machine-generated by time.Format, never arbitrary input, so
// it gets its own tight validation instead of widening that shared,
// security-sensitive charset.
var doltDateTimeLiteralPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)

// dateTimeLiteral quotes a dolt space-form datetime (from wmToDolt) as a SQL
// literal after verifying it matches that exact machine-generated shape.
func dateTimeLiteral(doltWM string) (string, error) {
	if !doltDateTimeLiteralPattern.MatchString(doltWM) {
		return "", fmt.Errorf("invalid dolt datetime literal %q", doltWM)
	}
	return "'" + doltWM + "'", nil
}

// mergeDelta applies changed onto prev (upsert by id; a tombstoned id is
// dropped instead), deduping by id, and returns the merged full set.
func mergeDelta(prev, changed []FullIssue) []FullIssue {
	merged := make(map[string]FullIssue, len(prev)+len(changed))
	for _, iss := range prev {
		merged[iss.ID] = iss
	}
	for _, iss := range changed {
		if iss.Status == "tombstone" {
			delete(merged, iss.ID)
			continue
		}
		merged[iss.ID] = iss
	}
	out := make([]FullIssue, 0, len(merged))
	for _, iss := range merged {
		out = append(out, iss)
	}
	return out
}

// validateRequired rejects a skeleton row missing any of the fields the rest
// of the app assumes are always present.
func validateRequired(iss FullIssue) error {
	if iss.ID == "" || iss.Title == "" || iss.Status == "" || iss.IssueType == "" {
		return appErrors.New(appErrors.CodeInvariant,
			fmt.Sprintf("dolt row missing required field(s): id=%q title=%q status=%q issue_type=%q",
				iss.ID, iss.Title, iss.Status, iss.IssueType), nil)
	}
	return nil
}

// validateSkeletonNotPreloaded is a production runtime guard (not just a
// test) against ab-6irx.2 regressing: a skeleton row must leave
// Comments==nil and DetailLoaded==false. internal/ui's
// markExportedCommentsLoaded treats Comments!=nil as "already loaded" and
// transferCommentState then discards genuinely-loaded comments on the very
// next refresh — exactly what happened when this constructor used to set
// Comments: []Comment{} unconditionally. If a future edit reintroduces a
// non-nil placeholder (or sets DetailLoaded early) here, this fails loudly
// at read time instead of silently corrupting the UI's comment/detail cache.
func validateSkeletonNotPreloaded(iss FullIssue) error {
	if iss.Comments != nil || iss.DetailLoaded {
		return appErrors.New(appErrors.CodeInvariant,
			fmt.Sprintf("dolt skeleton row %q must not be pre-marked loaded: comments=%v detailLoaded=%v (regresses ab-6irx.2)",
				iss.ID, iss.Comments, iss.DetailLoaded), nil)
	}
	return nil
}

// attachLabels loads all labels and appends each to its owning issue in byID.
// Labels for unknown issue ids (not present in byID) are silently skipped.
func (c *doltClient) attachLabels(ctx context.Context, asof string, byID map[string]*FullIssue) error {
	rows, err := c.r.query(ctx, "SELECT issue_id,label FROM labels"+asof+" ORDER BY issue_id,label")
	if err != nil {
		return err
	}
	for _, row := range rows {
		if iss, ok := byID[str(row["issue_id"])]; ok {
			iss.Labels = append(iss.Labels, str(row["label"]))
		}
	}
	return nil
}

// attachDeps loads all dependency rows (every dependency type, not just
// parent-child) and populates both the forward Dependencies edge on the
// depending issue and the reverse Dependents edge on the depended-upon issue.
func (c *doltClient) attachDeps(ctx context.Context, asof string, byID map[string]*FullIssue) error {
	rows, err := c.r.query(ctx, "SELECT issue_id,type,depends_on_issue_id FROM dependencies"+asof)
	if err != nil {
		return err
	}
	for _, row := range rows {
		from := str(row["issue_id"])
		to := str(row["depends_on_issue_id"])
		typ := str(row["type"])
		if iss, ok := byID[from]; ok {
			iss.Dependencies = append(iss.Dependencies, Dependency{TargetID: to, Type: typ})
		}
		if rev, ok := byID[to]; ok {
			rev.Dependents = append(rev.Dependents, Dependent{ID: from, Type: typ})
		}
	}
	return nil
}

// str converts a decoded JSON value (from a map[string]any row) to a string,
// treating any non-string value (including nil for SQL NULL) as "".
func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// intOf converts a decoded JSON value to an int. encoding/json decodes JSON
// numbers as float64 when unmarshaled into map[string]any, so that is the
// primary case; int is handled for direct/test-constructed values, and any
// other type (including nil) yields 0.
func intOf(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

// detailCols lists the heavy issue columns loaded lazily by loadDetail,
// separate from skeletonCols so a full issue listing avoids fetching large
// text fields for issues the user never opens.
const detailCols = "id,description,design,notes,acceptance_criteria,close_reason,external_ref"

// loadDetail fetches the heavy detail fields and comments for iss (already
// populated by skeleton) as of asof, and sets DetailLoaded=true on success.
func (c *doltClient) loadDetail(ctx context.Context, asof string, iss *FullIssue) error {
	lit, err := sqlLiteral(iss.ID)
	if err != nil {
		return err
	}
	rows, err := c.r.query(ctx, "SELECT "+detailCols+" FROM issues"+asof+" WHERE id="+lit)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		row := rows[0]
		iss.Description = str(row["description"])
		iss.Design = str(row["design"])
		iss.Notes = str(row["notes"])
		iss.AcceptanceCriteria = str(row["acceptance_criteria"])
		iss.CloseReason = str(row["close_reason"])
		iss.ExternalRef = str(row["external_ref"])
	}

	comments, err := c.Comments(ctx, iss.ID)
	if err != nil {
		return err
	}
	iss.Comments = comments
	iss.DetailLoaded = true
	return nil
}

// Comments implements Reader: it loads all comments for issueID ordered by
// creation time, returning an empty (never nil) slice when there are none.
func (c *doltClient) Comments(ctx context.Context, issueID string) ([]Comment, error) {
	lit, err := sqlLiteral(issueID)
	if err != nil {
		return nil, err
	}
	rows, err := c.r.query(ctx,
		"SELECT id,issue_id,author,text,created_at FROM comments WHERE issue_id="+lit+" ORDER BY created_at,id")
	if err != nil {
		return nil, err
	}
	out := make([]Comment, 0, len(rows))
	for _, row := range rows {
		out = append(out, Comment{
			ID:        str(row["id"]),
			IssueID:   str(row["issue_id"]),
			Author:    str(row["author"]),
			Text:      str(row["text"]),
			CreatedAt: normalizeDoltTime(str(row["created_at"])),
		})
	}
	return out, nil
}
