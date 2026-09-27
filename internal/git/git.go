// This tracks a Git repo that represents a Homesick Castle, and allows
// executing Git command inside that repo using -C to run it as if it
// were the working directory.
// We don't use a Go Git library, because we want to just rely on the user's
// own Git config, for example GPG signing etc.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type Repo struct {
	Path string
}

func New(path string) *Repo {
	return &Repo{Path: path}
}

// Builds a Git command for later execution, the caller decides how to handle stdout/stderr,
// depending on whether they just want to capture the output, or hand control over to the terminal
// for commands like commit and push.
func (repo *Repo) Command(args ...string) *exec.Cmd {
	return exec.Command("git", append([]string{"-C", repo.Path}, args...)...)
}

// normal runMode means the command runs in the foreground, with prompts passed through
// background runMode means the command runs in the background, with prompts hidden, and
// a timeout applied.
type runMode int

const (
	normal runMode = iota
	background
)

var backgroundTimeout = 20 * time.Second

// Runs the specified Git command, capturing stdout or stderr if the command errored out.
func (repo *Repo) run(mode runMode, args ...string) (string, error) {
	ctx := context.Background()

	if mode == background {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, backgroundTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo.Path}, args...)...)

	if mode == background {
		// Creates a new session detached from the current terminal, so our UI stays in control.
		// Only needed for background comands.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		// By default, the timeout kills only git. If Git is running SSH commands at the time,
		// output pipes are left open, so we kill the whole process group to ensure everything
		// is tidied up.
		cmd.Cancel = func() error {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}
