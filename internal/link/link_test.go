package link

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rylon/homesync/internal/castle"
)

// castleFixture builds a fake $HOME containing a fake castle, mirroring
// a typical homesick setup.
type castleFixture struct {
	t          *testing.T
	home       string
	testCastle castle.Castle
}

func newFixture(t *testing.T) *castleFixture {
	t.Helper()
	// We need to get the real path to the temp dir, because on macOS, `t.TempDir()` returns
	// a path in `/var`, which is actually a symlink to `/private/var`, otherwise
	// the tests will fail when comparing paths.
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(castle.ReposDir(home), "dotfiles")
	for _, dir := range []string{".git", "home"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	return &castleFixture{
		t:    t,
		home: home,
		testCastle: castle.Castle{
			Name: "dotfiles",
			Root: root,
		},
	}
}

// castleFile writes a file inside the fake castle.
func (fixture *castleFixture) castleFile(rel, content string) string {
	fixture.t.Helper()
	path := filepath.Join(fixture.testCastle.Home(), rel)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fixture.t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fixture.t.Fatal(err)
	}

	return path
}

// castleDir creates an empty directory inside the fake castle.
func (fixture *castleFixture) castleDir(rel string) string {
	fixture.t.Helper()
	path := filepath.Join(fixture.testCastle.Home(), rel)

	if err := os.MkdirAll(path, 0o755); err != nil {
		fixture.t.Fatal(err)
	}

	return path
}

// homeFile writes a real file into the fake $HOME.
func (fixture *castleFixture) homeFile(rel, content string) string {
	fixture.t.Helper()
	path := filepath.Join(fixture.home, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fixture.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fixture.t.Fatal(err)
	}
	return path
}

// homeLink creates a symlink in the fake $HOME pointing at target.
func (fixture *castleFixture) homeLink(rel, destination string) string {
	fixture.t.Helper()
	path := filepath.Join(fixture.home, rel)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fixture.t.Fatal(err)
	}

	if err := os.Symlink(destination, path); err != nil {
		fixture.t.Fatal(err)
	}

	return path
}

func (fixture *castleFixture) linker(subdirs ...string) *Linker {
	return &Linker{
		Castle:         fixture.testCastle,
		HomeDir:        fixture.home,
		Subdirs:        subdirs,
		AllCastleRoots: []string{fixture.testCastle.Root},
	}
}

func (fixture *castleFixture) plan(subdirs ...string) []Action {
	fixture.t.Helper()
	actions, err := fixture.linker(subdirs...).Plan()
	if err != nil {
		fixture.t.Fatal(err)
	}
	return actions
}

func find(t *testing.T, actions []Action, rel string) Action {
	t.Helper()
	for _, action := range actions {
		if action.Rel == rel {
			return action
		}
	}
	t.Fatalf("no action for %q; got %s", rel, summarise(actions))
	return Action{}
}

func absent(t *testing.T, actions []Action, rel string) {
	t.Helper()
	for _, action := range actions {
		if action.Rel == rel {
			t.Fatalf("want no action for %q, got kind %v", rel, action.Kind)
		}
	}
}

func summarise(actions []Action) string {
	out := ""
	for _, action := range actions {
		out += "\n  " + action.Kind.String() + " " + action.Rel
	}
	if out == "" {
		return "(no actions)"
	}
	return out
}

func TestPlanCreatesMissingLink(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.castleFile(".zshrc", "export A=1\n")

	got := find(t, fixture.plan(), ".zshrc")

	if got.Kind != Create {
		t.Errorf("Kind = %v, want Create", got.Kind)
	}
	if got.Source != source {
		t.Errorf("Source = %q, want %q", got.Source, source)
	}
	if want := filepath.Join(fixture.home, ".zshrc"); got.Destination != want {
		t.Errorf("Destination = %q, want %q", got.Destination, want)
	}
}

func TestPlanRecognisesCorrectLinkAsIdentical(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.castleFile(".zshrc", "export A=1\n")
	fixture.homeLink(".zshrc", source)

	if got := find(t, fixture.plan(), ".zshrc"); got.Kind != Identical {
		t.Errorf("Kind = %v, want Identical", got.Kind)
	}
}

func TestPlanReportsRealFileAsConflict(t *testing.T) {
	fixture := newFixture(t)
	fixture.castleFile(".zshrc", "from castle\n")
	fixture.homeFile(".zshrc", "from home\n")

	if got := find(t, fixture.plan(), ".zshrc"); got.Kind != Conflict {
		t.Errorf("Kind = %v, want Conflict", got.Kind)
	}
}

func TestPlanReportsForeignLinkAsSymlinkConflict(t *testing.T) {
	fixture := newFixture(t)
	fixture.castleFile(".zshrc", "from castle\n")
	elsewhere := fixture.homeFile("somewhere-else", "other\n")
	fixture.homeLink(".zshrc", elsewhere)

	got := find(t, fixture.plan(), ".zshrc")

	if got.Kind != SymlinkConflict {
		t.Errorf("Kind = %v, want SymlinkConflict", got.Kind)
	}
	if got.CurrentTarget != elsewhere {
		t.Errorf("CurrentTarget = %q, want %q", got.CurrentTarget, elsewhere)
	}
}

// A directory not listed in the `.homesick_subdir` is just linked "as is", so
// the child folders don't need any special treatment.
func TestPlanLinksUnlistedDirectoryWhole(t *testing.T) {
	fixture := newFixture(t)
	fixture.castleFile("bin/script.sh", "#!/bin/sh\n")

	got := find(t, fixture.plan(), "bin")

	if got.Kind != Create {
		t.Errorf("Kind = %v, want Create", got.Kind)
	}
	absent(t, fixture.plan(), filepath.Join("bin", "script.sh"))
}

// A directory that *is* listed in the `.homesick_subdir` is treated differently,
// we instead have to walk the subdir and link its children individually.
func TestPlanWalksListedSubdirInsteadOfLinkingIt(t *testing.T) {
	fixture := newFixture(t)
	fixture.castleFile(".exampleapp/settings.json", "{}\n")
	fixture.castleFile(".exampleapp/README.md", "# Example\n")

	actions := fixture.plan(".exampleapp")

	absent(t, actions, ".exampleapp")
	for _, rel := range []string{".exampleapp/settings.json", ".exampleapp/README.md"} {
		if got := find(t, actions, rel); got.Kind != Create {
			t.Errorf("%s: Kind = %v, want Create", rel, got.Kind)
		}
	}
}

// Listing .config/exampletool also excludes .config, to preserve the behaviour of the
// gem, which ignores any ancestor of a listed subdir.
func TestPlanIgnoresAncestorsOfListedSubdir(t *testing.T) {
	fixture := newFixture(t)
	fixture.castleFile(".config/exampletool/config", "theme = dark\n")

	actions := fixture.plan(".config", ".config/exampletool")

	absent(t, actions, ".config")
	absent(t, actions, ".config/exampletool")
	if got := find(t, actions, ".config/exampletool/config"); got.Kind != Create {
		t.Errorf("Kind = %v, want Create", got.Kind)
	}
}

// A listed subdir that is absent must not error, covers things like
// empty folders that Git would effectively "disappear" from the castle.
func TestPlanToleratesMissingSubdirDirectory(t *testing.T) {
	fixture := newFixture(t)
	fixture.castleFile(".zshrc", "export A=1\n")

	actions := fixture.plan(".config", ".config/exampletool")

	if got := find(t, actions, ".zshrc"); got.Kind != Create {
		t.Errorf("Kind = %v, want Create", got.Kind)
	}
}

// We must not attempt to link files back into the castle itself.
func TestPlanRefusesDestinationInsideCastleRoot(t *testing.T) {
	fixture := newFixture(t)
	fixture.castleFile(".homesick/repos/dotfiles/stray", "oops\n")

	actions := fixture.plan(".homesick/repos/dotfiles")

	got := find(t, actions, ".homesick/repos/dotfiles/stray")
	if got.Kind != Refused {
		t.Fatalf("Kind = %v, want Refused", got.Kind)
	}
	if got.Reason == "" {
		t.Error("want a Reason explaining the refusal, got empty string")
	}
}

func TestPlanToleratesEmptyListedSubdir(t *testing.T) {
	fixture := newFixture(t)
	fixture.castleDir(".homesick/repos/dotfiles")
	fixture.castleFile(".zshrc", "export A=1\n")

	actions := fixture.plan(".homesick/repos/dotfiles")

	if got := find(t, actions, ".zshrc"); got.Kind != Create {
		t.Errorf("Kind = %v, want Create", got.Kind)
	}
}

func TestApplyCreatesMissingLink(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.castleFile(".zshrc", "export A=1\n")
	linker := fixture.linker()

	actions, err := linker.Plan()
	if err != nil {
		t.Fatal(err)
	}
	res, err := linker.Apply(actions)
	if err != nil {
		t.Fatal(err)
	}

	if res.Created != 1 {
		t.Errorf("Created = %d, want 1", res.Created)
	}
	dest := filepath.Join(fixture.home, ".zshrc")
	target, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("want a symlink at %s: %v", dest, err)
	}
	if target != source {
		t.Errorf("link target = %q, want %q", target, source)
	}
}

func TestApplyCreatesParentDirectories(t *testing.T) {
	fixture := newFixture(t)
	fixture.castleFile(".config/exampletool/config", "theme = dark\n")
	linker := fixture.linker(".config", ".config/exampletool")

	actions, err := linker.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := linker.Apply(actions); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(fixture.home, ".config", "exampletool", "config")
	if _, err := os.Readlink(dest); err != nil {
		t.Fatalf("want a symlink at %s: %v", dest, err)
	}
	// The intermediate directories must be real, not symlinks into the castle.
	info, err := os.Lstat(filepath.Join(fixture.home, ".config"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("~/.config is a symlink, want a real directory")
	}
}

// The regression test for the footgun the skill warns about. `homesick link`
// with closed stdin takes the default Y and destroys this file.
func TestApplyLeavesConflictingFileUntouched(t *testing.T) {
	fixture := newFixture(t)
	fixture.castleFile(".zshrc", "from castle\n")
	dest := fixture.homeFile(".zshrc", "precious local edits\n")
	linker := fixture.linker()

	actions, err := linker.Plan()
	if err != nil {
		t.Fatal(err)
	}
	res, err := linker.Apply(actions)
	if err != nil {
		t.Fatal(err)
	}

	if res.Created != 0 {
		t.Errorf("Created = %d, want 0", res.Created)
	}
	if res.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", res.Skipped)
	}
	content, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "precious local edits\n" {
		t.Errorf("file content = %q, want it unchanged", content)
	}
	info, err := os.Lstat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("destination became a symlink, want the real file preserved")
	}
}

func TestApplyLeavesForeignSymlinkUntouched(t *testing.T) {
	fixture := newFixture(t)
	fixture.castleFile(".zshrc", "from castle\n")
	elsewhere := fixture.homeFile("somewhere-else", "other\n")
	dest := fixture.homeLink(".zshrc", elsewhere)
	linker := fixture.linker()

	actions, err := linker.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := linker.Apply(actions); err != nil {
		t.Fatal(err)
	}

	target, err := os.Readlink(dest)
	if err != nil {
		t.Fatal(err)
	}
	if target != elsewhere {
		t.Errorf("link target = %q, want it unchanged at %q", target, elsewhere)
	}
}

func TestApplyNeverActsOnRefusedAction(t *testing.T) {
	fixture := newFixture(t)
	fixture.castleFile(".homesick/repos/dotfiles/stray", "oops\n")
	linker := fixture.linker(".homesick/repos/dotfiles")

	actions, err := linker.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := linker.Apply(actions); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(fixture.testCastle.Root, "stray")
	if _, err := os.Lstat(dest); err == nil {
		t.Errorf("Apply created %s inside the castle root", dest)
	}
}

func TestOverwriteReplacesConflictingFile(t *testing.T) {
	fixture := newFixture(t)
	source := fixture.castleFile(".zshrc", "from castle\n")
	dest := fixture.homeFile(".zshrc", "local edits\n")
	linker := fixture.linker()

	actions, err := linker.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if err := linker.Overwrite(find(t, actions, ".zshrc")); err != nil {
		t.Fatal(err)
	}

	target, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("want a symlink at %s: %v", dest, err)
	}
	if target != source {
		t.Errorf("link target = %q, want %q", target, source)
	}
}

func TestOverwriteRefusesDestinationInsideCastleRoot(t *testing.T) {
	fixture := newFixture(t)
	fixture.castleFile(".homesick/repos/dotfiles/stray", "oops\n")
	linker := fixture.linker(".homesick/repos/dotfiles")

	// Build the action by hand as a Conflict, so the refusal cannot be
	// satisfied by the "not a conflict" check instead of the castle-root guard.
	action := Action{
		Kind:        Conflict,
		Rel:         ".homesick/repos/dotfiles/stray",
		Source:      filepath.Join(fixture.testCastle.Home(), ".homesick/repos/dotfiles/stray"),
		Destination: filepath.Join(fixture.testCastle.Root, "stray"),
	}

	err := linker.Overwrite(action)
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if !strings.Contains(err.Error(), "inside the castle") {
		t.Errorf("error = %q, want it to name the castle-root guard", err)
	}
}

func TestOverwriteRefusesNonConflictAction(t *testing.T) {
	fixture := newFixture(t)
	fixture.castleFile(".zshrc", "from castle\n")
	linker := fixture.linker()

	actions, err := linker.Plan()
	if err != nil {
		t.Fatal(err)
	}
	action := find(t, actions, ".zshrc")
	if action.Kind != Create {
		t.Fatalf("precondition: Kind = %v, want Create", action.Kind)
	}

	if err := linker.Overwrite(action); err == nil {
		t.Fatal("want an error for a non-conflict action, got nil")
	}
}
