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
