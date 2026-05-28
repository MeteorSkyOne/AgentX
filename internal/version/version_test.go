package version

import (
	"strings"
	"testing"
)

func TestShort(t *testing.T) {
	restoreVersionVars(t)

	Version = ""
	if got := Short(); got != "dev" {
		t.Fatalf("Short() = %q, want dev", got)
	}

	Version = " 1.2.3 "
	if got := Short(); got != "1.2.3" {
		t.Fatalf("Short() = %q, want 1.2.3", got)
	}
}

func TestInfoUsesUnknownFallbacks(t *testing.T) {
	restoreVersionVars(t)

	Version = "1.2.3"
	Commit = ""
	Date = ""
	got := Info()
	for _, want := range []string{"1.2.3", "commit unknown", "built unknown"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Info() = %q, want it to contain %q", got, want)
		}
	}
}

func TestIsDev(t *testing.T) {
	restoreVersionVars(t)

	for _, tc := range []struct {
		version string
		want    bool
	}{
		{version: "dev", want: true},
		{version: "0.1.0-dev.3", want: true},
		{version: "0.1.0-dirty", want: true},
		{version: "0.1.0", want: false},
	} {
		Version = tc.version
		if got := IsDev(); got != tc.want {
			t.Fatalf("IsDev() with Version=%q = %v, want %v", tc.version, got, tc.want)
		}
	}
}

func restoreVersionVars(t *testing.T) {
	t.Helper()
	oldVersion := Version
	oldCommit := Commit
	oldDate := Date
	t.Cleanup(func() {
		Version = oldVersion
		Commit = oldCommit
		Date = oldDate
	})
}
