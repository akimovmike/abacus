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
	r := &doltRunner{dir: "/x", run: stub, mu: &sync.Mutex{}}
	rows, err := r.query(context.Background(), "SELECT 1")
	if err != nil || len(rows) != 1 || fmt.Sprint(rows[0]["n"]) != "42" {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
}

func TestDoltRunnerQueryError(t *testing.T) {
	stub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return []byte("Error: syntax error near SELEC\nmore detail"), errors.New("exit status 1")
	}
	r := &doltRunner{dir: "/x", run: stub, mu: &sync.Mutex{}}
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
	r := &doltRunner{dir: "/x", run: stub, mu: &sync.Mutex{}}
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
	r := &doltRunner{dir: "/x", run: stub, mu: &sync.Mutex{}}

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
	r := &doltRunner{dir: "/x", run: stub, mu: &sync.Mutex{}}
	h, err := r.snapshot(context.Background())
	if err != nil || h != "abc123" {
		t.Fatalf("h=%q err=%v", h, err)
	}
	if asOf("abc123") != " AS OF 'abc123'" {
		t.Fatalf("asOf=%q", asOf("abc123"))
	}
}
