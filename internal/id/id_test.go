package id

import "testing"

func TestNewReturnsPrefixedID(t *testing.T) {
	got := New("usr")
	if len(got) <= len("usr_") {
		t.Fatalf("id too short: %q", got)
	}
	if got[:4] != "usr_" {
		t.Fatalf("id prefix = %q, want usr_", got[:4])
	}
}

func TestNewTokenIsLong(t *testing.T) {
	got := NewToken()
	if len(got) < 32 {
		t.Fatalf("token length = %d, want >= 32", len(got))
	}
}
