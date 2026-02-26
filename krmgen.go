package main

import (
	_ "embed"
	"log"

	"github.com/librucha/krmgen/cmd"
)

var version string
var commit string
var date string

func main() {
	if err := cmd.NewRootCommand(version).Execute(); err != nil {
		log.Fatal(err)
	}
}
