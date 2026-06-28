package beads

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	appErrors "abacus/internal/errors"
)

// brDoltClient reads issues from a Dolt-backed beads store via the br CLI.
// Mutations delegate to the embedded br CLI Writer.
//
// ponytail: br JSON shape unverified — confirm against real br before merge.
type brDoltClient struct {
	workDir string
	bin     string
	Writer
}

// NewBrDoltClient constructs a br CLI-JSON reader for Dolt-backed beads.
func NewBrDoltClient(workDir string, opts ...BrCLIOption) Client {
	appErrors.Must(strings.TrimSpace(workDir) != "", "NewBrDoltClient requires a non-empty workDir")
	opts = append(opts, WithBrWorkDir(workDir))
	return &brDoltClient{
		workDir: workDir,
		bin:     resolveBrBinary(opts),
		Writer:  NewBrCLIClient(opts...),
	}
}

// resolveBrBinary extracts the configured br binary from options.
func resolveBrBinary(opts []BrCLIOption) string {
	c := &brCLIClient{bin: "br"}
	for _, opt := range opts {
		opt(c)
	}
	return c.bin
}

func (c *brDoltClient) List(ctx context.Context) ([]LiteIssue, error) {
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

func (c *brDoltClient) Show(ctx context.Context, ids []string) ([]FullIssue, error) {
	if len(ids) == 0 {
		return []FullIssue{}, nil
	}
	args := append([]string{"show"}, ids...)
	args = append(args, "--json", "--include-comments", "--include-dependents")
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("run br show: %w", err)
	}
	return parseShowIssues(out)
}

func (c *brDoltClient) Export(ctx context.Context) ([]FullIssue, error) {
	out, err := c.run(ctx, "list", "--json", "--all", "--limit", "0")
	if err != nil {
		return nil, fmt.Errorf("run br list: %w", err)
	}
	jsonBytes := extractJSONArray(out)
	if jsonBytes == nil {
		return nil, fmt.Errorf("no JSON array found in br list output")
	}
	var issues []FullIssue
	if err := json.Unmarshal(jsonBytes, &issues); err != nil {
		return nil, fmt.Errorf("decode br list output: %w", err)
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

func (c *brDoltClient) Comments(ctx context.Context, issueID string) ([]Comment, error) {
	if strings.TrimSpace(issueID) == "" {
		return nil, fmt.Errorf("issue id is required for comments")
	}
	out, err := c.run(ctx, "show", issueID, "--json", "--include-comments")
	if err != nil {
		return nil, fmt.Errorf("run br show: %w", err)
	}
	jsonBytes := extractJSON(out)
	if jsonBytes == nil {
		return nil, fmt.Errorf("no JSON found in br show output")
	}
	var issue FullIssue
	if err := json.Unmarshal(jsonBytes, &issue); err != nil {
		return nil, fmt.Errorf("decode br show output: %w", err)
	}
	// ponytail: br comment JSON shape unverified — confirm against real br before merge.
	if issue.Comments == nil {
		return []Comment{}, nil
	}
	return issue.Comments, nil
}

func (c *brDoltClient) run(ctx context.Context, args ...string) ([]byte, error) {
	//nolint:gosec // G204: CLI wrapper intentionally shells out to br command
	cmd := exec.CommandContext(ctx, c.bin, args...)
	if c.workDir != "" {
		cmd.Dir = c.workDir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, formatBrCommandError(c.bin, args, err, out)
	}
	return out, nil
}

func (c *brDoltClient) guardNonEmptyCount(ctx context.Context) error {
	out, err := c.run(ctx, "count", "--json")
	if err != nil {
		return fmt.Errorf("run br count cross-check: %w", err)
	}
	jsonBytes := extractJSON(out)
	if jsonBytes == nil {
		return fmt.Errorf("no JSON found in br count output")
	}
	var result struct {
		Count         int `json:"count"`
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return fmt.Errorf("decode br count output: %w", err)
	}
	return appErrors.Require(result.Count <= 0,
		fmt.Sprintf("count mismatch: br list returned 0 issues but br count reports %d", result.Count))
}
