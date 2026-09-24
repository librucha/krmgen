package main

import (
	"os"
	"strings"
	"testing"
)

// docs/dockerhub.md is pushed to Docker Hub by the GoReleaser dockerhub pipe
// during a release, so anything that would fail there has to fail here first.
func TestDockerHubDescription_IsPublishable(t *testing.T) {
	content, err := os.ReadFile("docs/dockerhub.md")
	if err != nil {
		t.Fatalf("read Docker Hub description: %v", err)
	}
	text := string(content)

	// Docker Hub rejects a full description longer than 25,000 characters.
	if n := len([]rune(text)); n > 25000 {
		t.Errorf("description is %d characters, Docker Hub allows 25000", n)
	}
	// GoReleaser may evaluate the file as a Go template; krmgen's own
	// template examples would then break the release at the publish step.
	if strings.Contains(text, "{{") {
		t.Error("description contains Go template delimiters; link to the README for template examples instead")
	}
}
