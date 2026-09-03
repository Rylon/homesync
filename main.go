// Homesync is a terminal frontend for managing Homesick "castles", for syncing your dotfiles
// across multiple machines.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/Rylon/homesync/internal/castle"
	"github.com/Rylon/homesync/internal/ui"
)

func main() {
	reposFlag := flag.String("repos", "", "castle directory (default ~/.homesick/repos)")
	flag.Parse()

	if err := run(*reposFlag); err != nil {
		fmt.Fprintln(os.Stderr, "homesync:", err)
		os.Exit(1)
	}
}

func run(reposDir string) error {
	// Make sure the repos dir we got passed in exists...
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if reposDir == "" {
		reposDir = castle.ReposDir(homeDir)
	}

	// ...then discover all the castles within it.
	castles, err := castle.Discover(reposDir)
	if err != nil {
		return err
	}
	if len(castles) == 0 {
		return fmt.Errorf("no castles found in %s", reposDir)
	}

	// Then run Bubble Tea to handle the rest of the UI :)
	_, err = tea.NewProgram(ui.New(homeDir, castles)).Run()
	return err
}
