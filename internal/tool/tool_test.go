package tool

import (
	"runtime"
	"strings"
	"testing"
)

func TestRunCommand_CapturesStdout(t *testing.T) {
	stdOut, stdErr, err := RunCommand("echo", "hello")
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if strings.TrimSpace(stdOut) != "hello" {
		t.Errorf("stdOut = %q, want %q", strings.TrimSpace(stdOut), "hello")
	}
	if stdErr != "" {
		t.Errorf("stdErr = %q, want empty", stdErr)
	}
}

func TestRunCommand_CapturesStderrSeparately(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	stdOut, stdErr, err := RunCommand("sh", "-c", "echo out; echo err >&2")
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if strings.TrimSpace(stdOut) != "out" {
		t.Errorf("stdOut = %q, want %q", strings.TrimSpace(stdOut), "out")
	}
	if strings.TrimSpace(stdErr) != "err" {
		t.Errorf("stdErr = %q, want %q", strings.TrimSpace(stdErr), "err")
	}
}

func TestRunCommand_ReportsExitFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	_, stdErr, err := RunCommand("sh", "-c", "echo boom >&2; exit 3")
	if err == nil {
		t.Fatal("RunCommand() error = nil, want a non-zero exit error")
	}
	if strings.TrimSpace(stdErr) != "boom" {
		t.Errorf("stdErr = %q, want %q", strings.TrimSpace(stdErr), "boom")
	}
}

func TestRunCommand_ReportsMissingBinary(t *testing.T) {
	_, _, err := RunCommand("krmgen-no-such-binary-xyz")
	if err == nil {
		t.Fatal("RunCommand() error = nil, want an error for a missing binary")
	}
}
