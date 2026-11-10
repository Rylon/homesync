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
    // Snazzy "one-liner", though cargo fmt put it on multiple lines :D
    paths
        .iter() // Converts the slice into an iterator of &PathBufs.
        .filter_map(|path| check(path).err()) // Keeps only the files that failed validation.
        .collect() // Collects the problems into a Vec<Problem>.
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
    let directory = path.parent().unwrap_or(Path::new("."));
    let filename = path.file_name().unwrap_or(path.as_os_str());

    let cmd_output = Command::new("bash")
        .current_dir(directory)
        .arg("-n")
        .arg(filename)
        .output()?;

    // If bash succeeds, then the shell script is valid syntax.
    if cmd_output.status.success() {
        return Ok(());
    }

    // Otherwise we try to fetch the error text from stderr, or
    // just the exit code if stderr is empty.
    let stderr = String::from_utf8_lossy(&cmd_output.stderr);
    let error_message = if stderr.trim().is_empty() {
        cmd_output.status.to_string()
    } else {
        stderr.trim_end().to_owned()
    };

    Err(Reason::InvalidShell(error_message))
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    fn make_test_file(temp_dir: &TempDir, name: &str, contents: &str) -> PathBuf {
        let path = temp_dir.path().join(name);
        fs::write(&path, contents).unwrap();
        path
    }

    #[test]
    fn accepts_valid_json() {
        let temp_dir = TempDir::new().unwrap();
        let test_file = make_test_file(
            &temp_dir,
            "valid.json",
            r#"{"key": "value", "nested":{"key2": ["value1", "value2"]}}"#,
        );
        check(&test_file).unwrap();
    }

    #[test]
    fn rejects_truncated_json() {
        let temp_dir = TempDir::new().unwrap();
        let test_file = make_test_file(&temp_dir, "invalid.json", r#"{"key": "value""#);

        let problem = check(&test_file).unwrap_err();
        assert!(matches!(problem.reason, Reason::InvalidJson(_)));
        assert!(problem.to_string().contains("invalid.json"));
    }

    #[test]
    fn rejects_json_with_trailing_comma() {
        let temp_dir = TempDir::new().unwrap();
        let test_file = make_test_file(&temp_dir, "invalid.json", r#"{"key": "value",}"#);

        let problem = check(&test_file).unwrap_err();
        assert!(matches!(problem.reason, Reason::InvalidJson(_)));
        assert!(problem.to_string().contains("invalid.json"));
    }

    #[test]
    fn rejects_json_with_git_conflict_markers() {
        let temp_dir = TempDir::new().unwrap();
        let test_file = make_test_file(
            &temp_dir,
            "invalid.json",
            "{\n<<<<<<< HEAD\n\"a\": 1\n=======\n\"a\": 2\n>>>>>>> other\n}\n",
        );

        let problem = check(&test_file).unwrap_err();
        assert!(matches!(problem.reason, Reason::InvalidJson(_)));
        assert!(problem.to_string().contains("invalid.json"));
    }

    #[test]
    fn ignores_unknown_file_extensions() {
        let temp_dir = TempDir::new().unwrap();
        let test_file = make_test_file(&temp_dir, "unknown_file_extension.mystery", "{");
        check(&test_file).unwrap();
    }

    #[test]
    fn ignores_missing_files() {
        let temp_dir = TempDir::new().unwrap();
        // Here we make sure a missing file doesn't cause an error. We use a file extension that
        // would normally result in a check, otherwise we wouldn't know if the behaviour was
        // correct, or the file was being skipped because it didn't have a checkable file extension.
        check(&temp_dir.path().join("does_not_exist.json")).unwrap();
    }

    #[test]
    fn accepts_valid_shell_script() {
        let temp_dir = TempDir::new().unwrap();
        let test_file =
            make_test_file(&temp_dir, "valid.sh", "#!/bin/bash\necho \"Hello, world!\"");
        check(&test_file).unwrap();
    }

    #[test]
    fn rejects_invalid_shell_script() {
        let temp_dir = TempDir::new().unwrap();
        let test_file = make_test_file(
            &temp_dir,
            "invalid.sh",
            "#!/bin/bash\nif [ -n \"$HOME\" ]; then echo \"HOME is set to $HOME!\n",
        );

        let problem = check(&test_file).unwrap_err();
        assert!(matches!(problem.reason, Reason::InvalidShell(_)));
        assert!(problem.to_string().contains("invalid.sh"));
    }

    #[test]
    fn handles_bash_as_a_file_extension() {
        let temp_dir = TempDir::new().unwrap();
        let test_file = make_test_file(
            &temp_dir,
            "valid.bash",
            "#!/bin/bash\necho \"Hello, world!\"",
        );
        check(&test_file).unwrap();

        let test_file = make_test_file(
            &temp_dir,
            "invalid.bash",
            "#!/bin/bash\necho \"Hello, world!",
        );
        let problem = check(&test_file).unwrap_err();
        assert!(matches!(problem.reason, Reason::InvalidShell(_)));
        assert!(problem.to_string().contains("invalid.bash"));
    }

    #[test]
    fn check_all_reports_every_problem() {
        let temp_dir = TempDir::new().unwrap();

        let valid_json = make_test_file(&temp_dir, "valid.json", "{}");
        let invalid_json = make_test_file(&temp_dir, "invalid.json", "{");
        let valid_bash = make_test_file(
            &temp_dir,
            "valid.bash",
            "#!/bin/bash\necho \"Hello, world!\"",
        );
        let invalid_bash =
            make_test_file(&temp_dir, "invalid.bash", "#!/bin/bash\necho \"Hello, !");

        let paths = vec![valid_json, invalid_json, valid_bash, invalid_bash];
        let problems = check_all(&paths);

        assert_eq!(problems.len(), 2);
        assert_eq!(problems[0].path, paths[1]);
        assert_eq!(problems[1].path, paths[3]);
    }

    #[test]
    fn check_all_reports_no_problems_for_valid_files() {
        let temp_dir = TempDir::new().unwrap();

        let valid_json = make_test_file(&temp_dir, "valid.json", "{}");
        let valid_bash = make_test_file(
            &temp_dir,
            "valid.bash",
            "#!/bin/bash\necho \"Hello, world!\"",
        );

        let paths = vec![valid_json, valid_bash];
        let problems = check_all(&paths);

        assert!(problems.is_empty());
    }
}
