package beads

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSQLLiteral(t *testing.T) {
	ok, err := sqlLiteral("ab-6irx")
	if err != nil || ok != "'ab-6irx'" {
		t.Fatalf("got %q err=%v, want 'ab-6irx'", ok, err)
	}
	// single quote is doubled
	q, err := sqlLiteral("a'b")
	if err != nil || q != "'a''b'" {
		t.Fatalf("got %q err=%v, want 'a''b'", q, err)
	}
	// injection / metachars rejected
	for _, bad := range []string{"a;DROP", "a--", "a b", "a\"b", "a\x00b"} {
		if _, err := sqlLiteral(bad); err == nil {
			t.Errorf("expected reject for %q", bad)
		}
	}
}

func TestParseDoltRows(t *testing.T) {
	rows, err := parseDoltRows([]byte(`{"rows":[{"id":"ab-1","priority":3}]}`))
	if err != nil || len(rows) != 1 || rows[0]["id"] != "ab-1" {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	// empty result is {} with no rows key -> zero rows, no error
	empty, err := parseDoltRows([]byte(`{}`))
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty=%v err=%v", empty, err)
	}
	// leading warning preamble is stripped
	pre, err := parseDoltRows([]byte("Warning: something\n{\"rows\":[{\"id\":\"ab-2\"}]}"))
	if err != nil || len(pre) != 1 {
		t.Fatalf("pre=%v err=%v", pre, err)
	}
}

func TestNormalizeDoltTime(t *testing.T) {
	got := normalizeDoltTime("2026-01-20 18:53:52")
	if got != "2026-01-20T18:53:52Z" {
		t.Fatalf("got %q", got)
	}
	if normalizeDoltTime("") != "" {
		t.Fatal("empty should stay empty")
	}
}

func TestResolveDoltDirRejectsTraversal(t *testing.T) {
	base := t.TempDir()
	// no embeddeddolt/x -> error
	if _, err := resolveDoltDir(base, "x"); err == nil {
		t.Fatal("expected error for missing dir")
	}
	if _, err := resolveDoltDir(base, "../escape"); err == nil {
		t.Fatal("expected error for traversal")
	}
}

func TestResolveDoltDirSuccess(t *testing.T) {
	base := t.TempDir()
	dbDir := filepath.Join(base, "embeddeddolt", "beads")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveDoltDir(base, "beads")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantReal, err := filepath.EvalSymlinks(dbDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != wantReal {
		t.Fatalf("got %q, want %q", got, wantReal)
	}
}

// TestResolveDoltDirRejectsEscapeViaCleanPath exercises the case where the
// joined+cleaned path resolves to a directory that exists but sits outside
// beadsDir, as opposed to simply not existing.
func TestResolveDoltDirRejectsEscapeViaCleanPath(t *testing.T) {
	parent := t.TempDir()
	base := filepath.Join(parent, "beadsdir")
	if err := os.MkdirAll(filepath.Join(base, "embeddeddolt"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	// "../../outside" resolves (after Join+Clean against
	// base/embeddeddolt/../../outside) to parent/outside: one ".." cancels
	// "embeddeddolt", the second escapes "beadsdir" itself. That directory
	// exists but is not under base.
	if _, err := resolveDoltDir(base, "../../outside"); err == nil {
		t.Fatal("expected error for path escaping beadsDir")
	}
}

func TestResolveDoltDirRejectsSymlinkEscape(t *testing.T) {
	parent := t.TempDir()
	base := filepath.Join(parent, "beadsdir")
	if err := os.MkdirAll(filepath.Join(base, "embeddeddolt"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "embeddeddolt", "beads")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if _, err := resolveDoltDir(base, "beads"); err == nil {
		t.Fatal("expected error for symlink escaping beadsDir")
	}
}

func TestDoltRunnerQuery(t *testing.T) {
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return []byte(`{"rows":[{"n":42}]}`), nil
	}
	r := &doltRunner{dir: "/x", run: stub, sem: make(chan struct{}, 1)}
	rows, err := r.query(context.Background(), "SELECT 1")
	if err != nil || len(rows) != 1 || fmt.Sprint(rows[0]["n"]) != "42" {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
}

func TestDoltRunnerQueryError(t *testing.T) {
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return []byte("Error: syntax error near SELEC\nmore detail"), errors.New("exit status 1")
	}
	r := &doltRunner{dir: "/x", run: stub, sem: make(chan struct{}, 1)}
	_, err := r.query(context.Background(), "SELEC 1")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "syntax error near SELEC") {
		t.Fatalf("expected error to include first line of output, got %q", got)
	}
}

func TestDoltRunnerQueryAppliesTimeout(t *testing.T) {
	var gotDeadline time.Time
	var hasDeadline bool
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		gotDeadline, hasDeadline = ctx.Deadline()
		return []byte(`{}`), nil
	}
	r := &doltRunner{dir: "/x", run: stub, sem: make(chan struct{}, 1)}
	before := time.Now()
	if _, err := r.query(context.Background(), "SELECT 1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasDeadline {
		t.Fatal("expected run to receive a context with a deadline")
	}
	if d := gotDeadline.Sub(before); d <= 0 || d > doltQueryTimeout+time.Second {
		t.Fatalf("deadline %v not within expected timeout window", d)
	}
}

func TestDoltRunnerQuerySerializesCalls(t *testing.T) {
	var active int32
	var maxActive int32
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		n := atomic.AddInt32(&active, 1)
		for {
			m := atomic.LoadInt32(&maxActive)
			if n <= m || atomic.CompareAndSwapInt32(&maxActive, m, n) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		return []byte(`{}`), nil
	}
	r := &doltRunner{dir: "/x", run: stub, sem: make(chan struct{}, 1)}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.query(context.Background(), "SELECT 1"); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Fatalf("expected calls to be serialized (max concurrent = 1), got %d", got)
	}
}

// TestDoltRunnerQueryRespectsCtxWhileWaitingForLock guards ab-6irx.5: a
// caller blocked waiting for a busy store must return ctx.Err() at the
// caller's deadline instead of blocking past it. This is driven by holding
// r.sem externally (simulating another in-flight query) and giving query a
// short-timeout ctx; a plain sync.Mutex would ignore ctx entirely and block
// until the holder released the lock.
func TestDoltRunnerQueryRespectsCtxWhileWaitingForLock(t *testing.T) {
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return []byte(`{}`), nil
	}
	r := &doltRunner{dir: "/x", run: stub, sem: make(chan struct{}, 1)}
	// Hold the lock externally so query() must wait for it.
	r.sem <- struct{}{}
	defer func() { <-r.sem }()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := r.query(ctx, "SELECT 1")
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded while waiting for a busy lock, got %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("query blocked for %v waiting on a busy lock, want a prompt ctx.Err() return", elapsed)
	}
}

// TestDoltRunnerQueryRetriesOnLockBusy guards ab-6irx.4: a dolt invocation
// that fails with a lock-busy signature (the exact "cannot update
// manifest: database is read only" message reproduced empirically against
// real dolt 2.1.10 by racing concurrent `dolt sql` writers, see
// isDoltLockBusy's doc comment) is retried rather than surfaced as a
// failed read.
func TestDoltRunnerQueryRetriesOnLockBusy(t *testing.T) {
	var calls int32
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return []byte("error on line 1 for query SELECT 1: cannot update manifest: database is read only"),
				errors.New("exit status 1")
		}
		return []byte(`{"rows":[{"n":42}]}`), nil
	}
	r := &doltRunner{dir: "/x", run: stub, sem: make(chan struct{}, 1)}

	rows, err := r.query(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatalf("expected eventual success after retries, got error: %v", err)
	}
	if len(rows) != 1 || fmt.Sprint(rows[0]["n"]) != "42" {
		t.Fatalf("rows=%v", rows)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("expected 3 attempts (2 retries before success), got %d", got)
	}
}

// TestDoltRunnerQueryExhaustsRetriesOnPersistentLockBusy guards the
// exhaustion path: once retries run out, the last (lock-busy) error is
// returned wrapped, not silently swallowed.
func TestDoltRunnerQueryExhaustsRetriesOnPersistentLockBusy(t *testing.T) {
	var calls int32
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return []byte("error on line 1 for query SELECT 1: cannot update manifest: database is read only"),
			errors.New("exit status 1")
	}
	r := &doltRunner{dir: "/x", run: stub, sem: make(chan struct{}, 1)}

	_, err := r.query(context.Background(), "SELECT 1")
	if err == nil {
		t.Fatal("expected an error once retries are exhausted")
	}
	if !strings.Contains(err.Error(), "database is read only") {
		t.Fatalf("expected wrapped last error to mention the lock-busy message, got %v", err)
	}
	wantAttempts := int32(1 + len(doltLockBusyBackoffs))
	if got := atomic.LoadInt32(&calls); got != wantAttempts {
		t.Fatalf("expected %d attempts (1 initial + %d retries), got %d", wantAttempts, len(doltLockBusyBackoffs), got)
	}
}

// TestDoltRunnerQueryDoesNotRetryNonTransientError guards against wasting
// the retry budget (and hiding real failures) on an error that has nothing
// to do with a busy store, e.g. a genuine SQL syntax error.
func TestDoltRunnerQueryDoesNotRetryNonTransientError(t *testing.T) {
	var calls int32
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return []byte("Error: syntax error near SELEC\nmore detail"), errors.New("exit status 1")
	}
	r := &doltRunner{dir: "/x", run: stub, sem: make(chan struct{}, 1)}

	if _, err := r.query(context.Background(), "SELEC 1"); err == nil {
		t.Fatal("expected an error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected exactly 1 attempt (no retry for a non-transient error), got %d", got)
	}
}

// TestDoltRunnerQueryRetryRespectsCtxCancellation guards that a persistent
// lock-busy failure does not retry past the caller's ctx deadline: with a
// ctx that expires well before the full backoff schedule elapses, query
// must return promptly instead of exhausting every retry.
func TestDoltRunnerQueryRetryRespectsCtxCancellation(t *testing.T) {
	var calls int32
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return []byte("error on line 1 for query SELECT 1: cannot update manifest: database is read only"),
			errors.New("exit status 1")
	}
	r := &doltRunner{dir: "/x", run: stub, sem: make(chan struct{}, 1)}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := r.query(ctx, "SELECT 1")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error")
	}
	// Full backoff exhaustion (50+150+300ms) would take ~500ms; a 60ms ctx
	// must cut the retry loop short well before that.
	if elapsed > 400*time.Millisecond {
		t.Fatalf("query took %v to fail, want it to stop retrying once ctx (60ms) expired", elapsed)
	}
}

// TestIsDoltLockBusy pins the classifier's exact matching behavior: it must
// recognize the empirically-observed dolt manifest-contention message
// (case-insensitively) and must not misclassify an unrelated error.
func TestIsDoltLockBusy(t *testing.T) {
	busy := []byte("error on line 1 for query UPDATE t SET v=v+1 WHERE id=1: cannot update manifest: database is read only")
	if !isDoltLockBusy(busy) {
		t.Errorf("expected %q to classify as lock-busy", busy)
	}
	// Case-insensitive.
	if !isDoltLockBusy([]byte("DATABASE IS READ ONLY")) {
		t.Error("expected uppercase variant to classify as lock-busy")
	}
	notBusy := []byte("error on line 1 for query SELEC 1: syntax error near SELEC")
	if isDoltLockBusy(notBusy) {
		t.Errorf("expected %q to NOT classify as lock-busy", notBusy)
	}
}

func TestCheckDoltVersionTooOld(t *testing.T) {
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return []byte("dolt version 1.9.0"), nil
	}
	if err := checkDoltVersion(context.Background(), stub); err == nil {
		t.Fatal("expected too-old error")
	}
}

func TestCheckDoltVersionOK(t *testing.T) {
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return []byte("dolt version 2.1.0"), nil
	}
	if err := checkDoltVersion(context.Background(), stub); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckDoltVersionNotAvailable(t *testing.T) {
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return nil, errors.New(`exec: "dolt": executable file not found in $PATH`)
	}
	if err := checkDoltVersion(context.Background(), stub); err == nil {
		t.Fatal("expected error when dolt is unavailable")
	}
}

// TestCheckDoltVersionRespectsContextTimeout guards NewDoltClient's
// construction-time version gate (ab-6irx): checkDoltVersion must not hang
// past its caller's deadline. The stub simulates a wedged `dolt version`
// subprocess by blocking until ctx is done (mirroring how execDolt's
// exec.CommandContext would be killed on timeout), and this asserts that a
// short caller-supplied timeout makes checkDoltVersion return promptly with
// an error rather than blocking for the stub's full duration.
func TestCheckDoltVersionRespectsContextTimeout(t *testing.T) {
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
			return []byte("dolt version 2.1.0"), nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := checkDoltVersion(ctx, stub)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error when the context times out before the stub returns")
	}
	if elapsed > time.Second {
		t.Fatalf("checkDoltVersion took %v, want it to return promptly once ctx times out", elapsed)
	}
}

func TestFirstLine(t *testing.T) {
	cases := map[string]string{
		"":             "",
		"one line":     "one line",
		"line1\nline2": "line1",
		"line1\n":      "line1",
	}
	for input, want := range cases {
		if got := firstLine([]byte(input)); got != want {
			t.Errorf("firstLine(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSnapshotAsOf(t *testing.T) {
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return []byte(`{"rows":[{"h":"abc123"}]}`), nil
	}
	r := &doltRunner{dir: "/x", run: stub, sem: make(chan struct{}, 1)}
	h, err := r.snapshot(context.Background())
	if err != nil || h != "abc123" {
		t.Fatalf("h=%q err=%v", h, err)
	}
	if asOf("abc123") != " AS OF 'abc123'" {
		t.Fatalf("asOf=%q", asOf("abc123"))
	}
}
