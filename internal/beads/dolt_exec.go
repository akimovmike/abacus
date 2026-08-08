package beads

import (
	"fmt"
	"regexp"
	"strings"
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
