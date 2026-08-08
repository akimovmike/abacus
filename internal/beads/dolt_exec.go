package beads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// idCharset allows beads ids (ab-xyz, dotted like ab-pccw.3.15) and UUIDs.
var idCharset = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// sqlLiteralCharset allows idCharset chars plus single quotes; -- is checked separately.
var sqlLiteralCharset = regexp.MustCompile(`^[A-Za-z0-9._'-]+$`)

func validIssueID(id string) bool {
	return id != "" && len(id) <= 128 && idCharset.MatchString(id)
}

// sqlLiteral returns v as a safe single-quoted SQL string literal.
// Only allowlisted characters are permitted; single quotes are doubled.
func sqlLiteral(v string) (string, error) {
	// Reject -- (SQL comment syntax)
	if strings.Contains(v, "--") {
		return "", fmt.Errorf("value %q contains disallowed characters", v)
	}
	if !sqlLiteralCharset.MatchString(v) {
		return "", fmt.Errorf("value %q contains disallowed characters", v)
	}
	return "'" + strings.ReplaceAll(v, "'", "''") + "'", nil
}

func parseDoltRows(out []byte) ([]map[string]any, error) {
	if i := bytes.IndexByte(out, '{'); i > 0 { // strip any non-JSON preamble
		out = out[i:]
	}
	var envelope struct {
		Rows *[]map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		return nil, fmt.Errorf("decode dolt json: %w", err)
	}
	if envelope.Rows == nil {
		return []map[string]any{}, nil // {} == zero rows
	}
	return *envelope.Rows, nil
}

var doltTimeLayouts = []string{
	time.RFC3339Nano, time.RFC3339,
	"2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05",
}

func normalizeDoltTime(s string) string {
	if s == "" {
		return ""
	}
	for _, layout := range doltTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return s // leave as-is if unrecognized
}
