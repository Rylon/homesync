use std::ffi::OsStr;
use std::fmt;
use std::fs;
use std::io;
use std::path::{Path, PathBuf};
use std::process::Command;

#[derive(Debug)]
pub struct Problem {
    pub path: PathBuf,
    pub reason: Reason,
}

#[derive(Debug)]
pub enum Reason {
    Io(io::Error),
    InvalidJson(serde_json::Error),
    InvalidShell(String),
}

impl fmt::Display for Problem {
    fn fmt(&self, formatter: &mut fmt::Formatter) -> fmt::Result {
        let file_display_name = self
            .path
            .file_name()
            .unwrap_or(self.path.as_os_str())
            .display();

        match &self.reason {
            Reason::Io(err) => write!(formatter, "I/O error at {}: {}", file_display_name, err),
            Reason::InvalidJson(err) => {
                write!(formatter, "invalid JSON at {}: {}", file_display_name, err)
            }
            Reason::InvalidShell(msg) => write!(
                formatter,
                "invalid shell script at {}: {}",
                file_display_name, msg
            ),
        }
    }
}

impl From<io::Error> for Reason {
    fn from(error: io::Error) -> Self {
        Reason::Io(error)
    }
}

impl From<serde_json::Error> for Reason {
    fn from(error: serde_json::Error) -> Self {
        Reason::InvalidJson(error)
    }
}

pub fn check(path: &Path) -> Result<(), Problem> {
    check_file(path).map_err(|reason| Problem {
        path: path.to_path_buf(),
        reason,
    })
}

pub fn check_all(paths: &[PathBuf]) -> Vec<Problem> {
    todo!()
}

fn check_file(path: &Path) -> Result<(), Reason> {
    if !path.try_exists()? {
        return Ok(());
    }

    let file_extension = path
        .extension() // Option<&OsStr>: for example "JSON", or None.
        .and_then(OsStr::to_str) // Option<&str>: None if it isn't valid UTF-8.
        .map(str::to_ascii_lowercase); // Option<String>: for example "json".

    match file_extension.as_deref() {
        Some("json") => check_json(path),
        Some("sh" | "bash") => check_shell(path),
        Some(_) | None => Ok(()),
    }
}

fn check_json(path: &Path) -> Result<(), Reason> {
    let contents = fs::read_to_string(path)?;
    serde_json::from_str::<serde_json::Value>(&contents)?;
    Ok(())
}

fn check_shell(path: &Path) -> Result<(), Reason> {
    todo!()
}
