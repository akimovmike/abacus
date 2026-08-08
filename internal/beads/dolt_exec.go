package beads

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// idCharset allows beads ids (ab-xyz, dotted like ab-pccw.3.15) and UUIDs.
var idCharset = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// sqlLiteralCharset allows idCharset chars plus single quotes; -- is checked separately.
var sqlLiteralCharset = regexp.MustCompile(`^[A-Za-z0-9._'-]+$`)

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

// MinDoltVersion is the minimum supported Dolt CLI version.
const MinDoltVersion = "2.1.0"

// doltQueryTimeout bounds every dolt sql invocation.
const doltQueryTimeout = 30 * time.Second

// doltLockBusyBackoffs are the delays between retry attempts when a dolt
// sql invocation fails with a lock-busy error (see isDoltLockBusy). Up to
// len(doltLockBusyBackoffs) retries are attempted -- i.e. up to
// len(doltLockBusyBackoffs)+1 total dolt invocations -- all within the
// query's doltQueryTimeout budget, and cut short by ctx cancellation.
var doltLockBusyBackoffs = []time.Duration{50 * time.Millisecond, 150 * time.Millisecond, 300 * time.Millisecond}

// doltLockBusyMarkers are case-insensitive substrings of a failed dolt sql
// invocation's combined output that indicate the embedded store was
// transiently locked by a concurrent dolt process (e.g. an external bd/br
// write racing this read for the store's manifest) rather than a genuine
// query failure.
//
// Verified empirically (2026-08-08, dolt 2.1.10): in a scratch dolt repo,
// racing N concurrent `dolt sql -q "UPDATE ..."` processes against the same
// local (non-server) database directory reliably produced this exact
// message on the losers of the race to update the manifest:
//
//	error on line 1 for query UPDATE t SET v=v+1 WHERE id=1: cannot update
//	manifest: database is read only
//
// (11 of 12 concurrent writers failed with this message in one run; the
// remaining 1 won the race and succeeded.) The same stress test run with
// concurrent *read* (SELECT) statements never reproduced this or any other
// error (0 of 12 failures) -- doltRunner.query only ever issues reads, so
// in practice this retry is a narrow safety net for the case where a read's
// manifest refresh races a concurrent writer's manifest swap, rather than a
// commonly-hit path. No other lock-busy wording (e.g. a generic "database
// locked" message) was observed against this dolt version; if a future
// dolt version phrases the same condition differently, this allowlist will
// need a matching addition.
var doltLockBusyMarkers = []string{
	"cannot update manifest",
	"database is read only",
}

// isDoltLockBusy reports whether out (the combined stdout+stderr of a
// failed dolt sql invocation) matches a known lock-busy signature.
func isDoltLockBusy(out []byte) bool {
	lower := strings.ToLower(string(out))
	for _, marker := range doltLockBusyMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// commandRunner executes a dolt subcommand and returns its combined output.
// Tests inject a stub; execDolt shells out to the real dolt binary.
type commandRunner func(ctx context.Context, dir string, args ...string) ([]byte, error)

// execDolt runs the real dolt binary in dir as its own process group so a
// context timeout/cancellation kills the whole group, not just the parent
// process.
func execDolt(ctx context.Context, dir string, args ...string) ([]byte, error) {
	//nolint:gosec // G204: CLI wrapper intentionally shells out to dolt command
	cmd := exec.CommandContext(ctx, "dolt", args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return cmd.CombinedOutput()
}

// resolveDoltDir returns the absolute embedded-Dolt database directory for
// database under beadsDir (<beadsDir>/embeddeddolt/<database>), rejecting
// path traversal and symlinks that escape beadsDir.
func resolveDoltDir(beadsDir, database string) (string, error) {
	base, err := filepath.Abs(beadsDir)
	if err != nil {
		return "", fmt.Errorf("resolve beads dir: %w", err)
	}
	// Resolve beadsDir itself through any symlinks (e.g. macOS /var ->
	// /private/var) so the escape check below compares two paths that went
	// through the same normalization, not a resolved child against a raw parent.
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", fmt.Errorf("resolve beads dir: %w", err)
	}
	dir := filepath.Join(base, "embeddeddolt", database)
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("dolt dir not found: %w", err)
	}
	rel, err := filepath.Rel(realBase, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("dolt dir %q escapes workspace %q", real, realBase)
	}
	return real, nil
}

// checkDoltVersion verifies that the dolt CLI reachable via run satisfies
// MinDoltVersion.
func checkDoltVersion(ctx context.Context, run commandRunner) error {
	out, err := run(ctx, "", "version")
	if err != nil {
		return fmt.Errorf("dolt not available: %w", err)
	}
	got, _, err := parseSemver(string(out))
	if err != nil {
		return fmt.Errorf("parse dolt version: %w", err)
	}
	minVersion, _, err := parseSemver(MinDoltVersion)
	if err != nil {
		return fmt.Errorf("parse minimum dolt version: %w", err)
	}
	if got.compare(minVersion) < 0 {
		return fmt.Errorf("dolt %s below minimum %s", strings.TrimSpace(string(out)), MinDoltVersion)
	}
	return nil
}

// doltRunner serializes dolt sql invocations against a single embedded
// database directory.
type doltRunner struct {
	dir string
	run commandRunner
	sem chan struct{} // buffered cap 1; acts as a ctx-aware mutex (see query)
}

// query runs sql against the dolt database and returns the decoded rows.
//
// The per-store lock (sem) is acquired only after the timeout context is
// derived from ctx, and acquisition itself respects ctx.Done(): a caller
// blocked waiting on a busy store returns ctx.Err() at the deadline instead
// of blocking past it. A plain sync.Mutex.Lock() cannot be interrupted by
// context cancellation, so a buffered (cap 1) channel is used as a
// ctx-aware mutex instead (ab-6irx.5).
//
// A dolt invocation that fails with a lock-busy signature (isDoltLockBusy)
// -- e.g. a concurrent external bd/br write racing this read for the
// store's manifest -- is retried with backoff (doltLockBusyBackoffs) inside
// the same timeout budget; any other error is returned immediately
// (ab-6irx.4).
func (r *doltRunner) query(ctx context.Context, sqlText string) ([]map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, doltQueryTimeout)
	defer cancel()

	select {
	case r.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-r.sem }()

	var lastErr error
	for attempt := 0; ; attempt++ {
		out, err := r.run(ctx, r.dir, "sql", "-q", sqlText, "-r", "json")
		if err == nil {
			return parseDoltRows(out)
		}
		lastErr = fmt.Errorf("dolt sql failed (%s): %w", firstLine(out), err)
		if !isDoltLockBusy(out) {
			return nil, lastErr
		}
		if attempt >= len(doltLockBusyBackoffs) {
			// Diagnostic marker so an exhausted lock-busy retry is
			// identifiable at the call site/logs as sustained store
			// contention, not a random query failure (there is no
			// separate metrics/watchdog layer in this CLI/TUI process).
			return nil, fmt.Errorf("dolt sql lock-busy after %d attempts: %w", attempt+1, lastErr)
		}
		select {
		case <-time.After(doltLockBusyBackoffs[attempt]):
		case <-ctx.Done():
			return nil, fmt.Errorf("%w (last dolt error: %v)", ctx.Err(), lastErr)
		}
	}
}

// firstLine returns the first line of b, or all of b if it has no newline.
func firstLine(b []byte) string {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}

// snapshot returns the current HEAD commit hash for r's database, used to
// pin subsequent reads to a consistent point via asOf.
//
// Note: concurrent refreshes are NOT coalesced at this runner layer; each
// call serializes only via r.sem (one dolt invocation at a time). Single-flight
// coalescing of concurrent refreshes is added at the reader layer in Task 8
// (doltClient.refreshMu).
func (r *doltRunner) snapshot(ctx context.Context) (string, error) {
	rows, err := r.query(ctx, "SELECT HASHOF('HEAD') AS h")
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("no snapshot hash returned")
	}
	h, _ := rows[0]["h"].(string)
	if !idCharset.MatchString(h) {
		return "", fmt.Errorf("unexpected snapshot hash %q", h)
	}
	return h, nil
}

// asOf returns an " AS OF '<hash>'" clause for appending to a FROM clause,
// pinning a query to the commit produced by snapshot.
//
// Precondition: hash must already be validated by snapshot()'s idCharset
// check before this is called. sqlLiteral's error is deliberately discarded
// below: idCharset ([A-Za-z0-9._-]) is a strict subset of sqlLiteralCharset
// (adds only quote/dash handling), so a hash that passed snapshot()'s check
// can never fail sqlLiteral. If a future caller passes an unvalidated hash,
// this invariant no longer holds and the discarded error should be
// reconsidered.
func asOf(hash string) string {
	lit, _ := sqlLiteral(hash) // hash validated by snapshot()
	return " AS OF " + lit
}
