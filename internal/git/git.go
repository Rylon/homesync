// This tracks a Git repo that represents a Homesick Castle, and allows
// executing Git command inside that repo using -C to run it as if it
// were the working directory.
// We don't use a Go Git library, because we want to just rely on the user's
// own Git config, for example GPG signing etc.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
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

// Runs the specified Git command, capturing stdout or stderr if the command errored out.
func (repo *Repo) run(args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := repo.Command(args...)
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
