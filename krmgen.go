package main

import (
	_ "embed"
	cmd "github.com/librucha/krmgen/cmd"
	"github.com/librucha/krmgen/version"
	"log"
)

//go:embed version.txt
var versionFile string

func main() {
	version.AppVersion = versionFile
	if err := cmd.NewRootCommand().Execute(); err != nil {
		log.Fatal(err)
	}
}
