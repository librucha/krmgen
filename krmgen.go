package main

import (
	_ "embed"
	cmd "github.com/librucha/krmgen/cmd"
	"log"
)

//go:embed version.txt
var versionFile string

func main() {
	cmd.SetAppVersion(versionFile)
	if err := cmd.NewRootCommand().Execute(); err != nil {
		log.Fatal(err)
	}
}
