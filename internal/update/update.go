// Package update looks for a newer homesync on GitHub Releases, downloads it, verifies the checksum.,
// then replaces the old binary with the new one, when the user chooses to apply the update.
package update

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver/v3"
	selfupdate "github.com/creativeprojects/go-selfupdate"
)

const Repository = "Rylon/homesync"
const checksumsFile = "checksums.txt"
const binaryName = "homesync"

// Version is the running homesync version. GoReleaser stamps it at link time with
// `-X github.com/Rylon/homesync/internal/update.Version=...`, see `.goreleaser.yaml`.
var Version = "dev"

type Checker struct {
	Version string
	path    string
	updater *selfupdate.Updater
}

// NewChecker normally checks GitHub Releases, but we can override that with a custom server URL
// primarily for testing the full update flow.
func NewChecker(version, serverURL string) (Checker, error) {
	var source selfupdate.Source

	if serverURL != "" {
		var err error
		if source, err = selfupdate.NewHttpSource(selfupdate.HttpConfig{BaseURL: serverURL}); err != nil {
			return Checker{}, err
		}
	}

	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source:    source,
		Validator: &selfupdate.ChecksumValidator{UniqueFilename: checksumsFile},
	})
	if err != nil {
		return Checker{}, err
	}

	path, err := selfupdate.ExecutablePath()
	if err != nil {
		return Checker{}, err
	}

	return Checker{Version: version, path: path, updater: updater}, nil
}

type Release struct {
	Version     string
	URL         string
	Notes       string
	PublishedAt time.Time

	asset *selfupdate.Release
}

// IsDevMode tries to parse the version as semver, if that fails then we must be in dev mode.
// Update checks are disabled in dev mode.
func IsDevMode(version string) bool {
	_, err := semver.NewVersion(version)
	return err != nil
}

// Label is a version as shown to the user, for example v1.0.0, or just `dev`.
func Label(version string) string {
	if IsDevMode(version) {
		return version
	}
	return "v" + version
}

// Latest checks for the latest GitHub release, builds the Release struct for it, and
// reports whether it's considered newer than the running version or not.
func (checker Checker) Latest(ctx context.Context) (Release, bool, error) {
	if IsDevMode(checker.Version) {
		return Release{}, false, nil
	}

	release, found, err := checker.updater.DetectLatest(ctx, selfupdate.ParseSlug(Repository))
	if err != nil {
		return Release{}, false, err
	}

	if !found || !release.GreaterThan(checker.Version) {
		return Release{}, false, nil
	}

	return Release{
		Version:     release.Version(),
		URL:         release.URL,
		Notes:       release.ReleaseNotes,
		PublishedAt: release.PublishedAt,
		asset:       release,
	}, true, nil
}

// Apply downloads the release, verifies the checksum, then replaces the old binary with the new one.
func (checker Checker) Apply(ctx context.Context, release Release) error {
	if name := filepath.Base(checker.path); name != binaryName {
		return fmt.Errorf("the binary is installed as %q, but must be named %q to update itself", name, binaryName)
	}

	return checker.updater.UpdateTo(ctx, release.asset, checker.path)
}
