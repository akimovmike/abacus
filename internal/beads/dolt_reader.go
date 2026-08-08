package beads

import (
	appErrors "abacus/internal/errors"
	"context"
	"fmt"
	"strings"
	"sync"
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

// Show implements Reader: it takes a single snapshot, then queries only the
// requested ids directly (issues/labels/deps scoped via idInClause, see
// showSkeletons) instead of scanning the full tables, then loadDetail fills
// in the heavy fields and comments. Show does not take refreshMu: it
// snapshots independently of Export/List's single-flighted refresh.
func (c *doltClient) Show(ctx context.Context, ids []string) ([]FullIssue, error) {
	if len(ids) == 0 {
		return []FullIssue{}, nil
	}
	snap, err := c.r.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	asof := asOf(snap)

	idClause, err := idInClause(ids)
	if err != nil {
		return nil, err
	}
	_, ordered, err := c.showSkeletons(ctx, asof, idClause)
	if err != nil {
		return nil, err
	}

	out := make([]FullIssue, 0, len(ordered))
	for _, iss := range ordered {
		if err := c.loadDetail(ctx, asof, iss); err != nil {
			return nil, err
		}
		out = append(out, *iss)
	}
	return out, nil
}

// idInClause builds a parenthesized, comma-joined `IN (...)` fragment (e.g.
// "('ab-1','ab-2')"), quoting each id via sqlLiteral (injection-safe).
func idInClause(ids []string) (string, error) {
	lits := make([]string, len(ids))
	for i, id := range ids {
		lit, err := sqlLiteral(id)
		if err != nil {
			return "", err
		}
		lits[i] = lit
	}
	return "(" + strings.Join(lits, ",") + ")", nil
}

// showSkeletons is Show's targeted alternative to skeleton: issues, labels,
// and dependencies are all scoped to idClause (from idInClause) instead of a
// full-table scan. Tombstoned requested ids are excluded (skeleton's default
// filter). Dependencies are scoped by `issue_id IN idClause OR
// depends_on_issue_id IN idClause` so both edge directions land on the
// requested ids even when the other end of the edge is outside idClause.
func (c *doltClient) showSkeletons(
	ctx context.Context, asof, idClause string,
) (map[string]*FullIssue, []*FullIssue, error) {
	where := skeletonWhereClause(false, "id IN "+idClause)
	q := "SELECT " + skeletonCols + " FROM issues" + asof + " WHERE " + where + " ORDER BY created_at, id"
	issueRows, err := c.r.query(ctx, q)
	if err != nil {
		return nil, nil, err
	}
	byID, ordered, err := assembleSkeletonIssues(issueRows)
	if err != nil {
		return nil, nil, err
	}
	if err := c.attachLabels(ctx, asof, byID, "issue_id IN "+idClause); err != nil {
		return nil, nil, err
	}
	depWhere := "issue_id IN " + idClause + " OR depends_on_issue_id IN " + idClause
	if err := c.attachDeps(ctx, asof, byID, depWhere); err != nil {
		return nil, nil, err
	}
	return byID, ordered, nil
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

	byID, ordered, err := assembleSkeletonIssues(issueRows)
	if err != nil {
		return nil, err
	}

	if err := c.attachLabels(ctx, asof, byID, ""); err != nil {
		return nil, err
	}
	if err := c.attachDeps(ctx, asof, byID, ""); err != nil {
		return nil, err
	}
	if err := c.attachCommentFingerprints(ctx, asof, byID); err != nil {
		return nil, err
	}

	out := make([]FullIssue, len(ordered))
	for i, p := range ordered {
		out[i] = *p
	}
	return out, nil
}

// assembleSkeletonIssues converts issue rows (already filtered by the
// caller's SQL) into per-issue FullIssue skeletons, validated and indexed by
// id; shared by skeletonWhere (full scan) and showSkeletons (id-scoped).
func assembleSkeletonIssues(issueRows []map[string]any) (map[string]*FullIssue, []*FullIssue, error) {
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
			return nil, nil, err
		}
		if err := validateSkeletonNotPreloaded(iss); err != nil {
			return nil, nil, err
		}
		byID[iss.ID] = &iss
		ordered = append(ordered, &iss)
	}
	return byID, ordered, nil
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

// attachLabels loads labels (scoped by where, or all if "") and appends each
// to its owning issue in byID; unknown issue ids are silently skipped.
func (c *doltClient) attachLabels(ctx context.Context, asof string, byID map[string]*FullIssue, where string) error {
	q := "SELECT issue_id,label FROM labels" + asof
	if where != "" {
		q += " WHERE " + where
	}
	rows, err := c.r.query(ctx, q+" ORDER BY issue_id,label")
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

// attachDeps loads dependency rows (scoped by where, or all if ""; every
// dependency type, not just parent-child) and populates both the forward
// Dependencies edge on the depending issue and the reverse Dependents edge
// on the depended-upon issue.
func (c *doltClient) attachDeps(ctx context.Context, asof string, byID map[string]*FullIssue, where string) error {
	q := "SELECT issue_id,type,depends_on_issue_id FROM dependencies" + asof
	if where != "" {
		q += " WHERE " + where
	}
	rows, err := c.r.query(ctx, q)
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
