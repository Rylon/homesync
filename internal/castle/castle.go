// A Castle is a git repository under ~/.homesick/repos with a specific
// layout managed by the Homesick gem.
package castle

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Homesick hardcodes these locations, they aren't configurable.
const (
	// This is the subfolder expected to be in the castle repo, which
	// is linked into the user's $HOME directory.
	HomeDirName = "home"

	// This config file defines sub-folders within the castle which
	// aren't symlinked whole, but contain files which are symlinked individually.
	// This allows you to track certain files without being forced to commit
	// an entire directory which may contain unrelated files.
	SubdirFilename = ".homesick_subdir"

	// Expected to be found relative to the user's home.
	homesickDirName = ".homesick"
	reposDirName    = "repos"
)

type Castle struct {
	Name string // directory name within the homesick dir, for example "dotfiles"
	Root string // the git working tree, for example ~/.homesick/repos/dotfiles
}

// Home is the subfolder that maps onto $HOME, for example
// ~/.homesick/repos/<repo_name>/home
func (c Castle) Home() string {
	return filepath.Join(c.Root, HomeDirName)
}

// ReposDir returns the directory that holds all castles.
func ReposDir(homeDir string) string {
	return filepath.Join(homeDir, homesickDirName, reposDirName)
}

// Discover returns every castle under reposDir, sorted by name, skipping any
// special `.git` folders which might be present in the reposDir itself.
func Discover(reposDir string) ([]Castle, error) {
	var found []Castle

	err := filepath.WalkDir(reposDir, func(path string, entry fs.DirEntry, err error) error {

		// Make sure the reposDir exists in the first place.
		if err != nil {
			if path == reposDir && os.IsNotExist(err) {
				return nil
			}
			return err
		}

		// If this isn't a directory, skip!
		if !entry.IsDir() {
			return nil
		}

		// WalkDir starts with the root of the path, and we mustn't treat the root itself
		// as a castle, as that would mask the actual castles within, so we skip!
		if path == reposDir {
			return nil
		}

		// Lstat, not Stat: a submodule records .git as a file, not a directory.
		// We use Lstat as we don't want to blindly follow symlinks, we want to
		// check the actual filesystem entry and determine what it is.
		if _, statErr := os.Lstat(filepath.Join(path, ".git")); statErr != nil {
			return nil
		}

		found = append(found, Castle{
			Name: filepath.Base(path),
			Root: path,
		})

		return fs.SkipDir
	})

	// If we got any errors walking the reposDir directory, return that now.
	if err != nil {
		return nil, err
	}

	// Otherwise we sort the Castles by name, and return them.
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found, nil
}
