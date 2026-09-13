package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeReleases is a pretend GitHub release for testing the update flow locally.
type fakeReleases struct {
	requests atomic.Int32
	url      string
}

// The archive has to contain a file named after the expected binary, or the updater refuses it.
func tarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buffer bytes.Buffer
	zipper := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(zipper)

	if err := archive.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipper.Close(); err != nil {
		t.Fatal(err)
	}

	return buffer.Bytes()
}

// serveRelease publishes a release version `tag` containing `binary`. An empty checksum param will
// calculate the correct value, set a non-empty value to override for testing checksum validation.
func serveRelease(t *testing.T, tag string, binary []byte, checksum string) *fakeReleases {
	t.Helper()

	assetName := fmt.Sprintf("homesync_%s_%s_%s.tar.gz", tag[1:], runtime.GOOS, runtime.GOARCH)
	asset := tarGz(t, "homesync", binary)

	if checksum == "" {
		sum := sha256.Sum256(asset)
		checksum = hex.EncodeToString(sum[:])
	}
	checksums := fmt.Sprintf("%s  %s\n", checksum, assetName)

	manifest := fmt.Sprintf(`releases:
  - id: 1
    tag_name: %s
    url: https://example.com/releases/%s
    release_notes: |
      ## Changelog
      * Add self-update
    assets:
      - id: 10
        name: %s
        url: %s
      - id: 11
        name: checksums.txt
        url: checksums.txt
`, tag, tag, assetName, assetName)

	fake := &fakeReleases{}

	mux := http.NewServeMux()
	mux.HandleFunc("/Rylon/homesync/manifest.yaml", func(w http.ResponseWriter, r *http.Request) {
		fake.requests.Add(1)
		w.Write([]byte(manifest))
	})

	mux.HandleFunc("/Rylon/homesync/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		w.Write(asset)
	})

	mux.HandleFunc("/Rylon/homesync/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(checksums))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	fake.url = server.URL

	return fake
}

func newChecker(t *testing.T, version, serverURL string) Checker {
	t.Helper()

	checker, err := NewChecker(version, serverURL)
	if err != nil {
		t.Fatal(err)
	}
	return checker
}

func TestLatestOnlyOffersAStrictlyNewerRelease(t *testing.T) {
	cases := []struct {
		name      string
		published string
		running   string
		want      bool
	}{
		{"older build shows an update required", "v0.2.0", "0.1.0", true},
		{"same version shows nothing", "v0.2.0", "0.2.0", false},
		{"newer build also shows nothing", "v0.2.0", "0.3.0", false},
		{"make sure the semvar pieces are compared numerically - 1", "v0.9.10", "0.9.9", true},
		{"make sure the semvar pieces are compared numerically - 2", "v0.11.12", "0.10.11", true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fake := serveRelease(t, testCase.published, []byte("new binary"), "")
			checker := newChecker(t, testCase.running, fake.url)

			release, found, err := checker.Latest(context.Background())
			if err != nil {
				t.Fatal(err)
			}

			if found != testCase.want {
				t.Fatalf("found = %v, want %v", found, testCase.want)
			}

			if want := testCase.published[1:]; found && release.Version != want {
				t.Errorf("release.Version = %q, want %q", release.Version, want)
			}

			if found && !strings.Contains(release.Notes, "Add self-update") {
				t.Errorf("release.Notes = %q, want the published changelog", release.Notes)
			}
		})
	}
}

// Make sure DevMode builds don't attempt any updates at all.
func TestLatestSkipsDevBuildsWithoutContactingTheServer(t *testing.T) {
	fake := serveRelease(t, "v0.2.0", []byte("new binary"), "")

	for _, version := range []string{"dev", "", "not-a-version"} {
		checker := newChecker(t, version, fake.url)

		if !IsDevMode(version) {
			t.Errorf("IsDevMode(%q) = false, want true", version)
		}

		_, found, err := checker.Latest(context.Background())
		if err != nil || found {
			t.Errorf("version %q: found = %v, err = %v, want neither", version, found, err)
		}
	}

	if fake.requests.Load() != 0 {
		t.Errorf("server saw %d requests, want 0", fake.requests.Load())
	}
}

func TestApplyReplacesTheBinary(t *testing.T) {
	fake := serveRelease(t, "v0.2.0", []byte("new binary"), "")
	checker := newChecker(t, "0.1.0", fake.url)

	binary := filepath.Join(t.TempDir(), "homesync")
	if err := os.WriteFile(binary, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	release, found, err := checker.Latest(context.Background())
	if err != nil || !found {
		t.Fatalf("Latest: found = %v, err = %v", found, err)
	}

	checker.path = binary
	if err := checker.Apply(context.Background(), release); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Errorf("binary content = %q, want %q", got, "new binary")
	}
}

// An invalid checksum should abort the update process.
func TestApplyRefusesADownloadWithTheWrongChecksum(t *testing.T) {
	fake := serveRelease(t, "v0.2.0", []byte("tampered binary"), "0000000000000000000000000000000000000000000000000000000000000000")
	checker := newChecker(t, "0.1.0", fake.url)

	binary := filepath.Join(t.TempDir(), "homesync")
	if err := os.WriteFile(binary, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	release, found, err := checker.Latest(context.Background())
	if err != nil || !found {
		t.Fatalf("Latest: found = %v, err = %v", found, err)
	}

	checker.path = binary
	if err := checker.Apply(context.Background(), release); err == nil {
		t.Fatal("Apply accepted a download with the wrong checksum")
	}

	got, _ := os.ReadFile(binary)
	if string(got) != "old binary" {
		t.Errorf("binary was changed to %q, want it left as %q", got, "old binary")
	}
}

// GoReleaser always produces archives with a binary named `homesync`, so if a user renames
// the binary for whatever reason, it'll break auto updates. We need to make sure we handle this
// case, with a clear error message.
func TestApplyRefusesARenamedBinary(t *testing.T) {
	fake := serveRelease(t, "v0.2.0", []byte("new binary"), "")
	checker := newChecker(t, "0.1.0", fake.url)

	release, found, err := checker.Latest(context.Background())
	if err != nil || !found {
		t.Fatalf("Latest: found = %v, err = %v", found, err)
	}

	checker.path = filepath.Join(t.TempDir(), "hs")
	if err := checker.Apply(context.Background(), release); err == nil || !strings.Contains(err.Error(), `must be named "homesync"`) {
		t.Fatalf("Apply on a renamed binary returned %v, want an error naming the expected file", err)
	}
}

func TestLabelPrefixesReleasesOnly(t *testing.T) {
	cases := map[string]string{"0.2.0": "v0.2.0", "1.0.0-rc1": "v1.0.0-rc1", "dev": "dev", "": ""}

	for version, want := range cases {
		if got := Label(version); got != want {
			t.Errorf("Label(%q) = %q, want %q", version, got, want)
		}
	}
}
