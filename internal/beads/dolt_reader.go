package beads

import (
	appErrors "abacus/internal/errors"
	"context"
	"fmt"
)

// doltClient reads Beads data directly via `dolt sql` against an embedded
// Dolt database, delegating writes to an embedded CLI Writer.
type doltClient struct {
	r *doltRunner
	Writer
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
	issueRows, err := c.r.query(ctx,
		"SELECT "+skeletonCols+" FROM issues"+asof+" WHERE status != 'tombstone' ORDER BY created_at, id")
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
			Comments:     []Comment{},
		}
		if err := validateRequired(iss); err != nil {
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
