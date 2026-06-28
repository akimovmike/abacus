package errors

import "testing"

func TestRequire_ReturnsNilWhenTrue(t *testing.T) {
	if err := Require(true, "should not fire"); err != nil {
		t.Fatalf("Require(true) = %v, want nil", err)
	}
}

func TestRequire_ReturnsInvariantErrorWhenFalse(t *testing.T) {
	err := Require(false, "expected failure")
	if err == nil {
		t.Fatal("Require(false) = nil, want error")
	}
	if !IsCode(err, CodeInvariant) {
		t.Fatalf("expected code %q, got %q", CodeInvariant, CodeOf(err))
	}
	if err.Error() != "expected failure" {
		t.Fatalf("expected message %q, got %q", "expected failure", err.Error())
	}
}

func TestMust_DoesNothingWhenTrue(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Must(true) panicked: %v", r)
		}
	}()
	Must(true, "should not fire")
}

func TestMust_PanicsWhenFalse(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Must(false) did not panic")
		}
		msg, ok := r.(string)
		if !ok || msg != "expected panic" {
			t.Fatalf("expected panic message %q, got %v", "expected panic", r)
		}
	}()
	Must(false, "expected panic")
}
