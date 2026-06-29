package beads

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"abacus/internal/debug"
	appErrors "abacus/internal/errors"
)

// bdDoltClient reads issues from a Dolt-backed beads store via the bd CLI.
// Mutations delegate to the embedded bd CLI Writer.
type bdDoltClient struct {
	workDir string
	bin     string
	Writer
}

// NewBdDoltClient constructs a bd CLI-JSON reader for Dolt-backed beads.
func NewBdDoltClient(workDir string, opts ...BdCLIOption) Client {
	appErrors.Must(strings.TrimSpace(workDir) != "", "NewBdDoltClient requires a non-empty workDir")
	opts = append(opts, WithBdWorkDir(workDir))
	return &bdDoltClient{
		workDir: workDir,
		bin:     resolveBdBinary(opts),
		Writer:  NewBdCLIClient(opts...),
	}
}

// resolveBdBinary extracts the configured bd binary from options.
func resolveBdBinary(opts []BdCLIOption) string {
	c := &bdCLIClient{bin: "bd"}
	for _, opt := range opts {
		opt(c)
	}
	return c.bin
}

func (c *bdDoltClient) List(ctx context.Context) ([]LiteIssue, error) {
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

func (c *bdDoltClient) Show(ctx context.Context, ids []string) ([]FullIssue, error) {
	if len(ids) == 0 {
		return []FullIssue{}, nil
	}
	args := append([]string{"show"}, ids...)
	args = append(args, "--json", "--include-comments", "--include-dependents")
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("run bd show: %w", err)
	}
	return parseShowIssues(out)
}

// parseShowIssues accepts either a single issue object or an array of issues.
func parseShowIssues(out []byte) ([]FullIssue, error) {
	if jsonBytes := extractJSONArray(out); jsonBytes != nil {
		var issues []FullIssue
		if err := json.Unmarshal(jsonBytes, &issues); err != nil {
			return nil, fmt.Errorf("decode bd show array: %w", err)
		}
		return issues, nil
	}
	if jsonBytes := extractJSON(out); jsonBytes != nil {
		var issue FullIssue
		if err := json.Unmarshal(jsonBytes, &issue); err != nil {
			return nil, fmt.Errorf("decode bd show object: %w", err)
		}
		return []FullIssue{issue}, nil
	}
	return nil, fmt.Errorf("no JSON found in bd show output")
}

func (c *bdDoltClient) Export(ctx context.Context) ([]FullIssue, error) {
	out, err := c.run(ctx, "list", "--json", "--all", "--limit", "0")
	if err != nil {
		return nil, fmt.Errorf("run bd list: %w", err)
	}
	jsonBytes := extractJSONArray(out)
	if jsonBytes == nil {
		return nil, fmt.Errorf("no JSON array found in bd list output")
	}
	var issues []FullIssue
	if err := json.Unmarshal(jsonBytes, &issues); err != nil {
		return nil, fmt.Errorf("decode bd list output: %w", err)
	}

	issues = filterTombstones(issues)

	if len(issues) == 0 {
		if crossErr := c.guardNonEmptyCount(ctx); crossErr != nil {
			return nil, crossErr
		}
		return []FullIssue{}, nil
	}

	if err := guardRequiredFields(issues[0]); err != nil {
		return nil, err
	}
	return issues, nil
}

func (c *bdDoltClient) Comments(ctx context.Context, issueID string) ([]Comment, error) {
	if strings.TrimSpace(issueID) == "" {
		return nil, fmt.Errorf("issue id is required for comments")
	}
	out, err := c.run(ctx, "show", issueID, "--json", "--include-comments")
	if err != nil {
		return nil, fmt.Errorf("run bd show: %w", err)
	}
	jsonBytes := extractJSON(out)
	if jsonBytes == nil {
		return nil, fmt.Errorf("no JSON found in bd show output")
	}
	var issue FullIssue
	if err := json.Unmarshal(jsonBytes, &issue); err != nil {
		return nil, fmt.Errorf("decode bd show output: %w", err)
	}
	// ponytail: bd comment JSON shape unverified — confirm against a repo with comments
	if issue.Comments == nil {
		return []Comment{}, nil
	}
	return issue.Comments, nil
}

func (c *bdDoltClient) run(ctx context.Context, args ...string) ([]byte, error) {
	finalArgs := make([]string, 0, len(args)+2)
	if c.workDir != "" {
		finalArgs = append(finalArgs, "-C", c.workDir)
	}
	finalArgs = append(finalArgs, args...)
	//nolint:gosec // G204: CLI wrapper intentionally shells out to bd command
	cmd := exec.CommandContext(ctx, c.bin, finalArgs...)
	if c.workDir != "" {
		cmd.Dir = c.workDir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		// ctx.Err() distinguishes a cancelled/timed-out run (process killed,
		// empty/partial output) from a genuine bd failure.
		debug.Logf("bd run failed: args=%v ctxErr=%v: %v", finalArgs, ctx.Err(), err)
		return nil, formatCommandError(c.bin, finalArgs, err, out)
	}
	return out, nil
}

func filterTombstones(issues []FullIssue) []FullIssue {
	out := make([]FullIssue, 0, len(issues))
	for _, iss := range issues {
		if iss.Status == "tombstone" {
			continue
		}
		out = append(out, iss)
	}
	return out
}

func guardRequiredFields(iss FullIssue) error {
	if strings.TrimSpace(iss.ID) == "" || strings.TrimSpace(iss.Title) == "" ||
		strings.TrimSpace(iss.Status) == "" || strings.TrimSpace(iss.IssueType) == "" {
		return appErrors.New(appErrors.CodeInvariant, "bd list returned issues with missing required fields", nil)
	}
	return nil
}

func (c *bdDoltClient) guardNonEmptyCount(ctx context.Context) error {
	out, err := c.run(ctx, "count", "--json")
	if err != nil {
		return fmt.Errorf("run bd count cross-check: %w", err)
	}
	jsonBytes := extractJSON(out)
	if jsonBytes == nil {
		return fmt.Errorf("no JSON found in bd count output")
	}
	var result struct {
		Count         int `json:"count"`
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return fmt.Errorf("decode bd count output: %w", err)
	}
	return appErrors.Require(result.Count <= 0,
		fmt.Sprintf("count mismatch: bd list returned 0 issues but bd count reports %d", result.Count))
}
