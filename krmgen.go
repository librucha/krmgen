package main

import (
	_ "embed"
	"os"

	"github.com/librucha/krmgen/cmd"
)

var version string

// commit and date are set via -X ldflags by goreleaser (see .goreleaser.yaml)
// during release builds. The linker doesn't see them as "used" since nothing
// in this package reads them, but they must stay for the release build to
// populate them (they are reserved for future use, e.g. in --version output).
//
//nolint:unused
var commit string

//nolint:unused
var date string

func main() {
	if err := cmd.NewRootCommand(version).Execute(); err != nil {
		os.Exit(1)
	}
}
