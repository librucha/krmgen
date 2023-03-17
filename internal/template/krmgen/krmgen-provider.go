package krmgen

import (
	"github.com/librucha/krmgen/version"
)

const VersionFunc = "krmgenVer"
const GeneratedFunc = "krmgenGenerated"

func ResolveKrmgenVersion() (string, error) {
	return version.AppVersion, nil
}

func ResolveKrmgenGenerated() (string, error) {
	return "krmgen-" + version.AppVersion, nil
}
