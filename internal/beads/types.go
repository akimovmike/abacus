package beads

import "encoding/json"

// LiteIssue represents a minimal issue record returned by `bd list`.
type LiteIssue struct {
	ID string `json:"id"`
}

// Comment models a Beads issue comment entry.
type Comment struct {
	ID        string `json:"id"`
	IssueID   string `json:"issue_id"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

// Dependency captures dependency metadata from the Beads API.
type Dependency struct {
	TargetID string `json:"id"`
	Type     string `json:"dependency_type"`
}

// UnmarshalJSON accepts either the "show" shape (id + dependency_type) or the
// "list" shape emitted by Dolt backends (depends_on_id + type).
func (d *Dependency) UnmarshalJSON(data []byte) error {
	var show struct {
		TargetID string `json:"id"`
		Type     string `json:"dependency_type"`
	}
	if err := json.Unmarshal(data, &show); err == nil && show.TargetID != "" {
		d.TargetID = show.TargetID
		d.Type = show.Type
		return nil
	}

	var list struct {
		TargetID string `json:"depends_on_id"`
		Type     string `json:"type"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	d.TargetID = list.TargetID
	d.Type = list.Type
	return nil
}

// Dependent represents a reverse dependency entry.
type Dependent struct {
	ID   string `json:"id"`
	Type string `json:"dependency_type"`
}

// FullIssue models the expanded issue data used by the UI.
type FullIssue struct {
	ID                 string       `json:"id"`
	Title              string       `json:"title"`
	Status             string       `json:"status"`
	IssueType          string       `json:"issue_type"`
	Priority           int          `json:"priority"`
	Description        string       `json:"description"`
	Design             string       `json:"design"`
	AcceptanceCriteria string       `json:"acceptance_criteria"`
	Notes              string       `json:"notes"`
	CreatedAt          string       `json:"created_at"`
	UpdatedAt          string       `json:"updated_at"`
	ClosedAt           string       `json:"closed_at"`
	CloseReason        string       `json:"close_reason"`
	ExternalRef        string       `json:"external_ref"`
	Assignee           string       `json:"assignee"`
	CreatedBy          string       `json:"created_by"`
	Labels             []string     `json:"labels"`
	Comments           []Comment    `json:"comments"`
	Dependencies       []Dependency `json:"dependencies"`
	Dependents         []Dependent  `json:"dependents"`
	// DetailLoaded reports whether the heavy detail fields (Description, Design,
	// Notes, AcceptanceCriteria, CloseReason) and Comments have been loaded.
	// Skeleton reads leave it false; the detail lazy-load sets it true.
	DetailLoaded bool `json:"-"`
	// CommentFingerprint is a cheap per-issue summary of the comments table
	// ("<count>|<max created_at>"), stamped by a skeleton read (dolt backend
	// only; see attachCommentFingerprints in dolt_delta.go) so a later
	// reconcile can detect an external comment add/edit without reloading
	// every issue's comments. Comments live in a separate table and don't
	// bump UpdatedAt, so this is the signal internal/ui's targeted cache
	// invalidation uses for the comment side (ab-6irx.6); UpdatedAt itself
	// already covers the detail side (description/notes/etc. all live on
	// the issues row). Left "" by backends that don't compute it (e.g.
	// sqlite, whose Export already returns real comments so this signal is
	// never needed there).
	CommentFingerprint string `json:"-"`
}
