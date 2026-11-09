use std::fs;
use std::io;
use std::path::{Path, PathBuf};

pub const CASTLE_HOME_DIR_NAME: &str = "home";
pub const SUBDIR_FILENAME: &str = ".homesick_subdir";
const HOMESICK_DIR_NAME: &str = ".homesick";
const REPOS_DIR_NAME: &str = "repos";

#[derive(Debug, Clone, PartialEq)]
pub struct Castle {
    pub name: String,
    pub root: PathBuf,
}

impl Castle {
    pub fn castle_home(&self) -> PathBuf {
        self.root.join(CASTLE_HOME_DIR_NAME)
    }
}

pub fn repos_dir(home_dir: &Path) -> PathBuf {
    home_dir.join(HOMESICK_DIR_NAME).join(REPOS_DIR_NAME)
}

pub fn discover(repos_dir: &Path) -> io::Result<Vec<Castle>> {
    // The repos directory might not exist yet, in which case we return an empty castle list.
    if !repos_dir.try_exists()? {
        return Ok(Vec::new());
    }

    let mut castles = Vec::new();
    find_castles(repos_dir, &mut castles)?;
    castles.sort_by(|a, b| a.name.cmp(&b.name));
    Ok(castles)
}

fn find_castles(dir: &Path, castles: &mut Vec<Castle>) -> io::Result<()> {
    for entry in fs::read_dir(dir)? {
        let entry = entry?;

        // Skip anything which isn't an actual directory (symlinked directories don't count)
        if !entry.file_type()?.is_dir() {
            continue;
        }

        let path = entry.path();

        // If we found something named ".git" at this location, we consider this a castle.
        if path.join(".git").symlink_metadata().is_ok() {
            // to_string_lossy() handles any non-UTF-8 characters in the directory name.
            let name = entry.file_name().to_string_lossy().into_owned();

            castles.push(Castle { name, root: path });
            continue;
        }

        // Otherwise keep searching, recursively.
        find_castles(&path, castles)?;
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    fn make_castle(temp_dir: &Path, name: &str) -> Castle {
        let castle_dir = temp_dir.join(name);
        for dir in [".git", CASTLE_HOME_DIR_NAME] {
            // Returns an io::Result, so unwrap will panic if there were errors
            // creating these dirs, to fail the tests.
            fs::create_dir_all(castle_dir.join(dir)).unwrap();
        }
        Castle {
            name: name.to_string(),
            root: castle_dir,
        }
    }

    #[test]
    fn discover_finds_a_single_castle() {
        let repos_dir = TempDir::new().unwrap();
        let castle = make_castle(repos_dir.path(), "my_castle");
        let discovered_castles = discover(repos_dir.path()).unwrap();

        // We should have found a single castle:
        //   * named "my_castle",
        //   * root of <repos_dir>/my_castle
        //   * castle_home() of <repos_dir>/my_castle/home
        assert_eq!(discovered_castles.len(), 1);
        assert_eq!(discovered_castles[0].name, "my_castle");
        assert_eq!(discovered_castles[0].root, castle.root);
        assert_eq!(
            discovered_castles[0].castle_home(),
            repos_dir.path().join("my_castle").join("home")
        );
    }

    #[test]
    fn discover_finds_four_castles_in_order() {
        let repos_dir = TempDir::new().unwrap();
        make_castle(repos_dir.path(), "castle_c");
        make_castle(repos_dir.path(), "castle_d");
        make_castle(repos_dir.path(), "castle_a");
        make_castle(repos_dir.path(), "castle_b");

        let discovered_castles = discover(repos_dir.path()).unwrap();
        // We should have found 4 castles, and they should be in alphabetical order.
        assert_eq!(discovered_castles.len(), 4);
        assert_eq!(discovered_castles[0].name, "castle_a");
        assert_eq!(discovered_castles[1].name, "castle_b");
        assert_eq!(discovered_castles[2].name, "castle_c");
        assert_eq!(discovered_castles[3].name, "castle_d");
    }

    #[test]
    fn discover_ignores_nested_git_folders() {
        let repos_dir = TempDir::new().unwrap();
        let castle = make_castle(repos_dir.path(), "my_castle");
        // Create a nested .git folder inside the castle
        fs::create_dir_all(castle.root.join("nested").join(".git")).unwrap();

        let discovered_castles = discover(repos_dir.path()).unwrap();
        // We should have found only the top-level castle, ignoring the nested .git folder.
        assert_eq!(discovered_castles.len(), 1);
        assert_eq!(discovered_castles[0].name, "my_castle");
        assert_eq!(discovered_castles[0].root, castle.root);
    }

    #[test]
    fn discover_supports_git_repos_using_gitfile() {
        let repos_dir = TempDir::new().unwrap();
        let castle = make_castle(repos_dir.path(), "my_castle");
        // Replace the .git folder the helper created with a `gitfile`, for testing
        // submodules, linked worktrees, or --separate-git-dir scenarios.
        fs::remove_dir(castle.root.join(".git")).unwrap();
        fs::write(castle.root.join(".git"), "gitdir: ../.git").unwrap();

        let discovered_castles = discover(repos_dir.path()).unwrap();

        // We should have found the castle even with a gitfile.
        assert_eq!(discovered_castles.len(), 1);
        assert_eq!(discovered_castles[0].name, "my_castle");
        assert_eq!(discovered_castles[0].root, castle.root);
    }

    #[test]
    fn discover_finds_a_git_repo_even_with_a_broken_symlink() {
        let repos_dir = TempDir::new().unwrap();
        let castle = make_castle(repos_dir.path(), "my_castle");

        // Replace the .git folder with a broken symlink
        fs::remove_dir(castle.root.join(".git")).unwrap();
        std::os::unix::fs::symlink("/non_existent_dir", castle.root.join(".git")).unwrap();

        let discovered_castles = discover(repos_dir.path()).unwrap();

        // We should have found the castle even with a broken symlink, this is to mostly to prevent
        // the castle from mysteriously disappearing due to a broken symlink, though it still wouldn't
        // be functional in this state.
        assert_eq!(discovered_castles.len(), 1);
        assert_eq!(discovered_castles[0].name, "my_castle");
        assert_eq!(discovered_castles[0].root, castle.root);
    }

    #[test]
    fn discover_ignores_folders_which_are_not_git_repos() {
        let repos_dir = TempDir::new().unwrap();

        // Make a real castle, and a fake castle, to avoid a false negative test if discovery was broken.
        make_castle(repos_dir.path(), "my_castle");
        let not_a_castle = repos_dir.path().join("not_a_castle");

        // Put what looks like a normal castle home folder in the fake castle, to double check it isn't
        // mistaken for a real castle by discovery.
        fs::create_dir_all(not_a_castle.join("home")).unwrap();

        let discovered_castles = discover(repos_dir.path()).unwrap();

        // We should only have found the real castle, and not the fake one.
        assert_eq!(discovered_castles.len(), 1);
        assert_eq!(discovered_castles[0].name, "my_castle");
    }

    #[test]
    fn discover_returns_no_castles_when_the_repos_dir_is_missing() {
        let repos_dir = TempDir::new().unwrap();
        let missing_repos_dir = repos_dir.path().join("missing");

        let discovered_castles = discover(&missing_repos_dir).unwrap();

        // We treat a missing repos folder as having found no castles, because this might be the
        // first time the user is running Homesync.
        assert_eq!(discovered_castles.len(), 0);
    }
}
