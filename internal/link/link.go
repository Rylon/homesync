// Package link plans and applies the symlinks that map a castle onto $HOME.
//
// This is a native Go replacement for `homesick link` from the Ruby gem. Plan
// and apply are separate steps. The interface shows what will happen before the
// user approves it, and each overwrite needs its own confirmation.
package link

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Rylon/homesync/internal/castle"
)

// Kind is what kind of action the linker will take with a given destination.
type Kind int

const (
	// In-practice we always set a kind, but we want to make sure the zero-value is inert.
	Unknown Kind = iota
	// Create means the destination is absent, so the symlink is safe to make.
	Create
	// Identical means the destination already points at the source.
	Identical
	// Conflict means a real file or directory occupies the destination.
	Conflict
	// SymlinkConflict means a symlink occupies the destination and points elsewhere.
	SymlinkConflict
	// Refused means the destination is inside a castle, which would create a loop.
	Refused
)

func (kind Kind) String() string {
	switch kind {
	case Unknown:
		return "unknown"
	case Create:
		return "create"
	case Identical:
		return "identical"
	case Conflict:
		return "conflict"
	case SymlinkConflict:
		return "symlink-conflict"
	case Refused:
		return "refused"
	}

	// This should never happen, so we recover with fmt to log what we ended up with here, for debugging.
	return fmt.Sprintf("Kind(%d)", int(kind))
}

// Action is one planned symlink.
type Action struct {
	Kind        Kind
	Rel         string // the file's identity: the same relative path as it appears in both the castle and the home directory
	Source      string // absolute, Rel inside the castle
	Destination string // absolute, Rel inside $HOME

	// CurrentTarget is where an existing symlink points. Only set for SymlinkConflict.
	CurrentTarget string

	// Reason explains a Refused action.
	Reason string
}

// Result reports what Apply did.
type Result struct {
	Created int
	Skipped int
	Errors  []error
}

// Linker maps one castle onto one home directory.
type Linker struct {
	Castle  castle.Castle
	HomeDir string
	Subdirs []string
	// the linker needs to know about all castle roots, so it can refuse to link a file into any of them.
	AllCastleRoots []string
}

// allCastleRoots returns the roots of every castle on the machine, or just the
// current castle, if there are no others.
func (linker *Linker) allCastleRoots() []string {
	if len(linker.AllCastleRoots) == 0 {
		return []string{linker.Castle.Root}
	}
	return linker.AllCastleRoots
}

// This handles the gem's `.homesick_subdir` file, which lists subfolders that are not linked
// in their entirety, but instead have specific files linked from within them.
// Useful for tracking parts of a subfolder that contains things that shouldn't be committed,
// like scratch files.
func (linker *Linker) ignoreSet() map[string]bool {
	ignore := make(map[string]bool)
	for _, subdir := range linker.Subdirs {
		for _, path := range selfAndAncestors(subdir) {
			ignore[path] = true
		}
	}
	return ignore
}

// selfAndAncestors lists a subdir and every directory above it, stopping before
// the top of the tree. For ".config/ghostty" it returns [".config/ghostty", ".config"].
func selfAndAncestors(subdir string) []string {
	var paths []string
	for path := filepath.Clean(subdir); path != "." && path != string(filepath.Separator); path = filepath.Dir(path) {
		paths = append(paths, path)
	}
	return paths
}

// insideRoot reports whether a given destination is inside any castle root.
func (linker *Linker) insideRoot(destination string) (string, bool) {
	for _, root := range linker.allCastleRoots() {
		rel, err := filepath.Rel(root, destination)
		if err != nil {
			continue
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return root, true
	}
	return "", false
}

// Plan checks the castle and home directory, to determine what actions are needed, if any.
func (linker *Linker) Plan() ([]Action, error) {
	ignore := linker.ignoreSet()
	bases := append([]string{"."}, linker.Subdirs...)

	var actions []Action
	for _, base := range bases {
		entries, err := os.ReadDir(filepath.Join(linker.Castle.Home(), base))

		if err != nil {
			// Git doesn't care about empty directories, so in some cases,
			// a subdir may be listed but not exist, so we need to skip
			// that to avoid a false error.
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		// Now we go through all the files in the castle, and classify what action is needed for each.
		for _, entry := range entries {
			rel := filepath.Join(base, entry.Name())
			if ignore[rel] {
				continue
			}
			action, err := linker.classify(rel)
			if err != nil {
				return nil, err
			}
			actions = append(actions, action)
		}
	}

	return actions, nil
}

// Classify determines what Action is needed for a given source path from the castle,
// based on the state of that destination path within the home folder.
func (linker *Linker) classify(rel string) (Action, error) {
	action := Action{
		Rel:         rel,
		Source:      filepath.Join(linker.Castle.Home(), rel),
		Destination: filepath.Join(linker.HomeDir, rel),
	}

	// Refuse to link a file into any castle, as that would create a loop and be confusing for the user.
	if root, inside := linker.insideRoot(action.Destination); inside {
		action.Kind = Refused
		action.Reason = fmt.Sprintf("destination is inside the castle at %s", root)
		return action, nil
	}

	info, err := os.Lstat(action.Destination)
	switch {

	case os.IsNotExist(err):
		action.Kind = Create

	case err != nil:
		return action, err

	// If the destination is a symlink, we check whether it already points at the correct source.
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(action.Destination)
		if err != nil {
			return action, err
		}

		resolved := target
		// We don't care whether the symlink is relative or absolute, we just want to know whether it points at the right place.
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(filepath.Dir(action.Destination), resolved)
		}
		// Then we do the comparison.
		if filepath.Clean(resolved) == action.Source {
			action.Kind = Identical
		} else {
			action.Kind = SymlinkConflict
			action.CurrentTarget = filepath.Clean(resolved)
		}

	default:
		// If we make it here then the destination already exists, and it's not a symlink, so we must have a conflict.
		action.Kind = Conflict
	}

	return action, nil
}

// Apply takes a list of Actions from Plan, and applies the desired changes.
// It returns a Result that reports how many were created, skipped, or failed.
func (linker *Linker) Apply(actions []Action) (Result, error) {
	var result Result
	for _, action := range actions {
		if action.Kind != Create {
			result.Skipped++
			continue
		}

		if _, inside := linker.insideRoot(action.Destination); inside {
			result.Skipped++
			continue
		}

		if err := os.MkdirAll(filepath.Dir(action.Destination), 0o755); err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}

		if err := os.Symlink(action.Source, action.Destination); err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}

		result.Created++
	}

	return result, nil
}

// When we have conflicts, the user must be prompted for each one, so we don't
// nuke a load of their files by accident, this function then gets called for
// each of their choices.
func (linker *Linker) Overwrite(action Action) error {

	// Guard against linking into a castle, and against overwriting a file that isn't a conflict.
	if root, inside := linker.insideRoot(action.Destination); inside {
		return fmt.Errorf("refusing to link %s: destination is inside the castle at %s", action.Rel, root)
	}
	if action.Kind != Conflict && action.Kind != SymlinkConflict {
		return fmt.Errorf("refusing to overwrite %s: action is %s, wasn't in conflict", action.Rel, action.Kind)
	}

	if err := os.RemoveAll(action.Destination); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(action.Destination), 0o755); err != nil {
		return err
	}

	return os.Symlink(action.Source, action.Destination)
}
