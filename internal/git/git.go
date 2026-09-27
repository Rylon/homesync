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
	"os"
	"os/exec"
	"strings"
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

const backgroundTimeout = 20 * time.Second

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
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND="+repo.batchSSHCommand())
		// ssh can outlive a killed git and hold the output pipes open, which would block `Wait`.
		cmd.WaitDelay = 2 * time.Second
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
