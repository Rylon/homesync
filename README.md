# homesync

Homesync is a command line app for managing your dotfiles. It keeps them in a Git repository, so you can sync them between computers, see the history of every change, and always have a backup.

It is inspired by and compatible with the excellent Ruby gem [Homesick](https://github.com/technicalpickles/homesick), and uses the same layout and config. Homesick calls the Git repo a "castle", so Homesync uses the same terminology.

## How does it work?

The files live inside the Git repo (the "castle"), and are symlinked into the correct location in your $HOME folder. Homesync then allows you to commit and push local changes, pull upstream changes from origin, and keeps the symlinks up to date.

### Feature parity

* Homesync has support for the following Homesick commands: `pull`, `commit`, `push`, `link`, `list`, `status`, and `diff`.

* It does not yet implement: `track`, `unlink`, `clone`, `generate`, `destroy`, `rc`, `open`, `exec`, `exec_all`, `show_path` or `cd`.

* There is also no onboarding yet - Homesync expects to find an existing castle on your machine.

* Homesync also runs basic validation of `.sh` and `.json` files, to prevent you from inadvertently pushing then pulling a malformed config file to another device.

### Git config

Homesync shells out to your native Git binary for interacting with the castle repo, so any config you have in place for commit signing, SSH keys, and so on, should Just Work™.

## Installing

Download the latest archive for your platform from the [releases page](https://github.com/Rylon/homesync/releases/latest), extract it, then move the `homesync` binary into a folder on your `$PATH`:

```sh
mkdir -p ~/.local/bin
tar -xf ~/Downloads/homesync_*_darwin_arm64.tar* -C ~/.local/bin homesync

# Ensures ~/.local/bin is on your $PATH, restart your shell to apply!
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zprofile

# Remove the quarantine flag for Apple Gatekeeper (see the note below).
xattr -d com.apple.quarantine ~/.local/bin/homesync
```

> [!NOTE]
> Homesync is not currently notarised by Apple, so you will see a warning from macOS when you try to run it:
>
> <img src="quarantine.png" width="306" alt="macOS dialog: homesync Not Opened. Apple could not verify homesync is free of malware.">
>
> To fix this, you can remove the quarantine flag, like so:
>
> ```sh
> xattr -d com.apple.quarantine ~/.local/bin/homesync
> ```

Archives are published for macOS and Linux, on both `amd64` and `arm64` architectures.

You're also welcome to download the source and compile it for yourself, see the [Local Development](#local-development) section below for instructions.

### Automatic updates

Homesync automatically checks for updates on launch. When new versions are available, you'll be able to read the changelog and apply the update from within the app.

Note: automatic updates are disabled when building from source.

## Local development

You need [Homebrew](https://brew.sh), and Go 1.27.0 or newer:

```
brew install go
git clone https://github.com/Rylon/homesync.git
cd homesync
go build
./homesync
```

### Tests

The test suite builds a fake $HOME and fake castle to test the behaviour properly, you can run all the tests like so:

```sh
go test ./...
```

### Releasing

GoReleaser handles creating new GitHub Releases automatically when pushing a new version tag, like so:

```sh
git tag -a v0.0.1 -m "New release!"
git push origin v0.0.1
```

You can also perform a snapshot build locally, which will compile everything, and write to `./dist/` for you to inspect.

```sh
brew install goreleaser
goreleaser release --snapshot --clean
```

## Known issues

### iTerm2: alternate screen mode scrollback

If you use iTerm2, the default behaviour is to save lines from the alternate screen mode to your scrollback. With this setting enabled, the terminal fills with fragments of the interface after you resize the window and quit.

If you don't need this behaviour, you can disable it to fix this:

1. From the iTerm2 menu, choose `Settings` -> `Profiles` -> `Terminal`.
2. Clear the checkbox for `Save lines to scrollback in alternate screen mode`.

I also tested using Apple Terminal and Ghostty, but they did not have this issue.
