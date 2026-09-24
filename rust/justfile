# `just check` runs every check CI runs. `just` alone lists the recipes.

set windows-shell := ["powershell.exe", "-NoLogo", "-Command"]

# Kept in one place: Cargo.toml.
msrv := replace_regex(read("Cargo.toml"), '(?s).*\nrust-version = "([^"]+)".*', '$1')

export RUSTDOCFLAGS := "-D warnings"

[private]
default:
    @just --list

# Every check, then the release build.
check: fmt clippy test doc deny machete msrv build

# Code formatted as rustfmt.toml says.
fmt:
    cargo fmt --check

# Lints from Cargo.toml's [lints], warnings as errors.
clippy:
    cargo clippy --all-targets --locked -- -D warnings

# The unit tests.
test:
    cargo test --locked

# Doc comments that point at missing items.
doc:
    cargo doc --no-deps --document-private-items --locked

# Known vulnerabilities, licences, banned crates and sources (deny.toml).
deny:
    cargo deny --locked check --hide-inclusion-graph

# Dependencies in Cargo.toml that nothing uses.
machete:
    cargo machete

# Builds with the oldest Rust that Cargo.toml promises.
msrv:
    rustup toolchain install {{msrv}} --profile minimal --no-self-update
    cargo +{{msrv}} check --all-targets --locked --target-dir target/msrv

# The release binary, in target/release/.
build:
    cargo build --release --locked

# Formats the code and applies clippy's safe suggestions.
fix:
    cargo fmt
    cargo clippy --all-targets --fix --allow-dirty --allow-staged

# The tools the checks need.
tools:
    rustup component add clippy rustfmt
    cargo install --locked cargo-deny cargo-machete
