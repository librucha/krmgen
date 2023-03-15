package tool

import (
	"bytes"
	"os/exec"
)

func RunCommand(name string, arg ...string) (stdOut string, stdErr string, err error) {
	cmd := exec.Command(name, arg...)
	var outBuffer bytes.Buffer
	var errBuffer bytes.Buffer
	cmd.Stdout = &outBuffer
	cmd.Stderr = &errBuffer
	runError := cmd.Run()
	return outBuffer.String(), errBuffer.String(), runError
}
