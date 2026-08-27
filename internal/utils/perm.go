package constants

import "os"

// FilePerm and DirPerm are the modes krmgen creates working files and
// directories with. Everything krmgen writes to its working directory is a
// rendered template, which may hold secrets pulled from a key vault, so
// nothing is readable outside the owning user.
const (
	FilePerm os.FileMode = 0600
	DirPerm  os.FileMode = 0700
)
