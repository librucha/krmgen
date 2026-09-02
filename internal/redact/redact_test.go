package redact

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCredentials(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "the reported kustomize error, where the URL appears twice",
			in: "accumulating resources from 'https://user:TOK@git.invalid/org/_git/repo?ref=master': " +
				"failed to run '/usr/bin/git fetch --depth=1 https://user:TOK@git.invalid/org/_git/repo master'",
			want: "accumulating resources from 'https://user:***@git.invalid/org/_git/repo?ref=master': " +
				"failed to run '/usr/bin/git fetch --depth=1 https://user:***@git.invalid/org/_git/repo master'",
		},
		{
			name: "azure devops personal access token",
			in:   "https://ORG:pat123@dev.azure.com/ORG/Service/_git/repo/apps/x?ref=master",
			want: "https://ORG:***@dev.azure.com/ORG/Service/_git/repo/apps/x?ref=master",
		},
		{
			// Invalid per RFC 3986, but a generated password can contain one.
			// The match is greedy so the whole password goes, not just the
			// part before the first "@".
			name: "unencoded @ inside the password",
			in:   "https://user:pa@ss@host/path",
			want: "https://user:***@host/path",
		},
		{
			name: "several URLs in one message",
			in:   "first https://a:1@x.io/ then https://b:2@y.io/",
			want: "first https://a:***@x.io/ then https://b:***@y.io/",
		},
		{
			// A username alone is not a credential, and dropping it would
			// make the message harder to act on.
			name: "username without a password",
			in:   "https://user@github.com/org/repo",
			want: "https://user@github.com/org/repo",
		},
		{
			name: "no credentials at all",
			in:   "reading https://charts.example.com/index.yaml failed",
			want: "reading https://charts.example.com/index.yaml failed",
		},
		{
			name: "oci chart reference",
			in:   "oci://registry-1.docker.io/library/chart",
			want: "oci://registry-1.docker.io/library/chart",
		},
		{
			// No "://", so there is no userinfo to find.
			name: "scp-style git remote",
			in:   "git@github.com:org/repo.git",
			want: "git@github.com:org/repo.git",
		},
		{
			// The "@" is in the path, not in a userinfo component.
			name: "at sign in the path",
			in:   "https://host/path@v2/file.yaml",
			want: "https://host/path@v2/file.yaml",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Credentials(tc.in); got != tc.want {
				t.Errorf("Credentials()\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

func TestError_NilStaysNil(t *testing.T) {
	if got := Error(nil); got != nil {
		t.Errorf("Error(nil) = %v, want nil", got)
	}
}

// An error with nothing to mask keeps its identity, so errors.Is and
// errors.As keep working for every error that is not a credential leak.
func TestError_UntouchedErrorKeepsItsIdentity(t *testing.T) {
	sentinel := errors.New("no kustomization file found")
	wrapped := fmt.Errorf("processing failed: %w", sentinel)

	got := Error(wrapped)
	if !errors.Is(got, sentinel) {
		t.Errorf("Error() = %v, want the original chain preserved", got)
	}
}

// A masked error must not keep the original one Unwrap away - that would put
// the secret back within reach of anything that walks the chain.
func TestError_MaskedErrorDoesNotCarryTheOriginal(t *testing.T) {
	const secret = "NOT-A-REAL-TOKEN-12345"
	sentinel := errors.New("fetch of https://user:" + secret + "@git.invalid/repo failed")
	wrapped := fmt.Errorf("processing failed: %w", sentinel)

	got := Error(wrapped)
	if got == nil {
		t.Fatal("Error() = nil, want the failure to survive masking")
	}
	if errors.Is(got, sentinel) {
		t.Error("the masked error still unwraps to the original, which still holds the secret")
	}
	for _, unwrapped := range []error{got, errors.Unwrap(got)} {
		if unwrapped != nil && strings.Contains(unwrapped.Error(), secret) {
			t.Errorf("the secret is still reachable: %v", unwrapped)
		}
	}
	if !strings.Contains(got.Error(), "https://user:***@git.invalid/repo") {
		t.Errorf("Error() = %v, want the masked URL", got)
	}
	if !strings.Contains(got.Error(), "processing failed") {
		t.Errorf("Error() = %v, want the surrounding message kept", got)
	}
}
