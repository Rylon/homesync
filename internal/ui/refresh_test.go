package ui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Rylon/homesync/internal/castle"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitIn(t *testing.T, dir, message string) {
	t.Helper()
	gitIn(t, dir, "-c", "user.email=test@example.org", "-c", "user.name=Test User", "-c", "commit.gpgsign=false",
		"commit", "--allow-empty", "-m", message)
}

// castleBehindOrigin builds a castle whose origin has one commit that the castle has not fetched yet.
func castleBehindOrigin(t *testing.T) Model {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, bare, clone := filepath.Join(dir, "castle"), filepath.Join(dir, "origin.git"), filepath.Join(dir, "clone")

	gitIn(t, dir, "init", "--bare", "-b", "main", bare)
	gitIn(t, dir, "init", "-b", "main", root)
	commitIn(t, root, "first")
	gitIn(t, root, "remote", "add", "origin", bare)
	gitIn(t, root, "push", "-u", "origin", "main")

	gitIn(t, dir, "clone", bare, clone)
	commitIn(t, clone, "from another machine")
	gitIn(t, clone, "push", "origin", "main")

	return New(dir, []castle.Castle{{Name: "castle", Root: root}}, releasedModel().checker)
}

func loadWith(t *testing.T, model Model, key string) loadedMsg {
	t.Helper()
	_, cmd := model.handleDashboardKey(key)
	msg, ok := cmd().(loadedMsg)
	if !ok {
		t.Fatalf("pressing %q did not produce a loadedMsg", key)
	}
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	return msg
}

func TestRefreshFetchesSoTheDashboardSeesNewCommitsOnOrigin(t *testing.T) {
	msg := loadWith(t, castleBehindOrigin(t), "r")

	if msg.behind != 1 {
		t.Errorf("behind = %d, want 1", msg.behind)
	}
	if msg.fetchErr != nil {
		t.Errorf("fetchErr = %v, want nil", msg.fetchErr)
	}
}

// With several castles, `Init` has no repo to fetch, so the fetch waits until the user picks one.
func TestChoosingACastleFetchesBeforeTheDashboardLoads(t *testing.T) {
	single := castleBehindOrigin(t)
	model := New(single.homeDir, []castle.Castle{single.castle, {Name: "other", Root: t.TempDir()}}, single.checker)

	_, cmd := model.handlePickerKey("enter")
	msg := cmd().(loadedMsg)

	if msg.behind != 1 {
		t.Errorf("behind = %d, want 1", msg.behind)
	}
}

// Reloads after a push or a relink only need the local state, so they must not wait on the network.
func TestReloadDoesNotFetch(t *testing.T) {
	_, cmd := castleBehindOrigin(t).reload()
	msg := cmd().(loadedMsg)

	if msg.behind != 0 {
		t.Errorf("behind = %d, want 0, as nothing was fetched", msg.behind)
	}
}

func TestRefreshStillLoadsTheCastleWhenTheFetchFails(t *testing.T) {
	model := castleBehindOrigin(t)
	gitIn(t, model.castle.Root, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))

	msg := loadWith(t, model, "r")

	if msg.fetchErr == nil {
		t.Error("want a fetchErr for an unreachable origin, got nil")
	}
	if msg.branch != "main" {
		t.Errorf("branch = %q, want the local state to load anyway", msg.branch)
	}
}

// Only a fetch can tell us if origin is reachable, so a local reload keeps the last fetch result.
func TestLoadedMsgKeepsTheFetchResultUntilTheNextFetch(t *testing.T) {
	failed := errors.New("could not read from remote repository")
	cases := []struct {
		name    string
		msg     loadedMsg
		wantErr error
	}{
		{"a failed fetch is recorded", loadedMsg{fetched: true, fetchErr: failed}, failed},
		{"a successful fetch clears it", loadedMsg{fetched: true}, nil},
		{"a local reload keeps it", loadedMsg{}, failed},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := releasedModel()
			model.fetchErr = failed

			next, _ := model.Update(testCase.msg)

			if got := next.(Model).fetchErr; got != testCase.wantErr {
				t.Errorf("fetchErr = %v, want %v", got, testCase.wantErr)
			}
		})
	}
}

func TestDashboardWarnsWhenTheFetchFailed(t *testing.T) {
	model := releasedModel()
	model.branch = "main"
	model.fetchErr = errors.New("git fetch --quiet origin: Permission denied (publickey)")

	view := ansi.Strip(model.viewDashboard())

	if !strings.Contains(view, "Permission denied (publickey)") {
		t.Errorf("dashboard does not show the fetch error:\n%s", view)
	}
}

func TestMain(m *testing.M) {
	// Stops the global Git config of the developer, such as a signing key, from affecting the test repos.
	os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	os.Exit(m.Run())
}

// A reload runs after every push and pull, so it must not wipe the error that tells the user why
// the push or pull failed.
func TestReloadKeepsTheErrorOfTheLastAction(t *testing.T) {
	cases := []struct {
		name  string
		start func() (tea.Model, tea.Cmd)
	}{
		{"a failed pull", func() (tea.Model, tea.Cmd) {
			return pullModel(pullIntegrating).updatePullMsg(pullReportMsg{err: errors.New("pull failed"), ahead: 1, behind: 1})
		}},
		{"a failed push", func() (tea.Model, tea.Cmd) {
			return pushModelWithOneFile().pushExecDone(execDoneMsg{label: "push", err: errors.New("rejected")})
		}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			next, _ := testCase.start()
			next, _ = next.(Model).Update(loadedMsg{})

			if next.(Model).err == nil {
				t.Error("the reload wiped the error")
			}
		})
	}
}

func TestReloadClearsItsOwnErrorOnceItWorks(t *testing.T) {
	model := releasedModel()

	next, _ := model.Update(loadedMsg{err: errors.New("not a git repository")})
	model = next.(Model)
	if model.loadErr == nil {
		t.Fatal("loadErr = nil, want the error from the failed reload")
	}
	if view := ansi.Strip(model.chrome("")); !strings.Contains(view, "not a git repository") {
		t.Errorf("the chrome does not show the reload error:\n%s", view)
	}

	next, _ = model.Update(loadedMsg{})
	if next.(Model).loadErr != nil {
		t.Errorf("loadErr = %v, want nil after a reload that worked", next.(Model).loadErr)
	}
}
