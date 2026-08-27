package castle

import (
	"os"
	"path/filepath"
	"strings"
)

// Folders listed in the special `.homesick_subdir` file are treated differently,
// rather than symlinking the whole thing, it's so you can track certain files within
// a subfolder, without having to commit the entire thing, which may contain unrelated files,
// or scratch files you shouldn't commit.
func Subdirs(castleRoot string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(castleRoot, SubdirFilename))

	// It may not exist if this feature isn't being used, so we handle that gracefully.
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var subdirs []string

	// Otherwise we gather all the non-empty lines, trimming whitespace, and return them.
	for _, line := range strings.Split(string(data), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			subdirs = append(subdirs, trimmed)
		}
	}

	return subdirs, nil
}
