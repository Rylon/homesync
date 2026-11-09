#[cfg(not(unix))]
compile_error!("homesync only supports macOS and Linux");

pub mod castle;
