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
