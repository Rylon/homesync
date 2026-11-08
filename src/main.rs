use homesync::castle;
use std::process;

fn main() {
    let Some(home_dir) = std::env::home_dir() else {
        eprintln!("homesync: error - could not determine your home directory.");
        process::exit(1);
    };

    let repos_dir = castle::repos_dir(&home_dir);

    let castles = match castle::discover(&repos_dir) {
        Ok(castles) => castles,
        Err(error) => {
            eprintln!(
                "homesync: error - failed to discover castles in {}: {error}",
                repos_dir.display()
            );
            process::exit(1);
        }
    };

    if castles.is_empty() {
        eprintln!(
            "homesync: error - no castles found in {}",
            repos_dir.display()
        );
        process::exit(1);
    }

    println!("Discovered castles:");
    for castle in &castles {
        println!("{} {}", castle.name, castle.root.display());
    }
}
