package krmgen

import (
	"strings"
	"testing"

	appVer "github.com/librucha/krmgen/version"
)

func TestResolveKrmgenVersion(t *testing.T) {
	original := appVer.AppVersion
	t.Cleanup(func() { appVer.AppVersion = original })
	appVer.AppVersion = "1.2.3"

	got, err := ResolveKrmgenVersion()
	if err != nil {
		t.Fatalf("ResolveKrmgenVersion() error = %v", err)
	}
	if got != "1.2.3" {
		t.Errorf("ResolveKrmgenVersion() = %q, want %q", got, "1.2.3")
	}
}

func TestResolveKrmgenGenerated(t *testing.T) {
	original := appVer.AppVersion
	t.Cleanup(func() { appVer.AppVersion = original })
	appVer.AppVersion = "1.2.3"

	got, err := ResolveKrmgenGenerated()
	if err != nil {
		t.Fatalf("ResolveKrmgenGenerated() error = %v", err)
	}
	if !strings.Contains(got, "1.2.3") {
		t.Errorf("ResolveKrmgenGenerated() = %q, want it to contain the version", got)
	}
}
