package beads

import "testing"

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

func TestValidIssueID(t *testing.T) {
	for _, id := range []string{"ab-6irx", "ab-pccw.3.15", "002c0cad-c1d2-518b-a3be-d07d1665c763"} {
		if !validIssueID(id) {
			t.Errorf("want valid: %q", id)
		}
	}
	for _, id := range []string{"", "a'b", "a;b", "a b", "a/b"} {
		if validIssueID(id) {
			t.Errorf("want invalid: %q", id)
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
