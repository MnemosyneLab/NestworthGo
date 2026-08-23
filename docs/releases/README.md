# Release Documents

This directory contains only release contracts and plans that have been
reviewed for the current Go + Fyne Nestworth-go implementation line.

## Active Releases

- [v0.1.1 release contract](v0.1.1.md) — implemented Household balance-sheet
  core; public-release closeout remains.
- [v0.1.2 release contract](v0.1.2.md) — Phases 0–9 implemented; release
  closeout is in progress.

## v0.1.2 Documents

- [v0.1.2 technical design](v0.1.2-technical-design.md)
- [v0.1.2 implementation plan](v0.1.2-implementation-plan.md)

v0.1.2 has been implemented for Go, Fyne, the current SQLite schema line, and
replaceable Yahoo and Frankfurter market-data boundaries. The release contract
and implementation plan remain the authority for the still-pending Phase 10
desktop, accessibility, signing, and distribution checks.

## Unreviewed Inherited Archive

The former Rust/Tauri v0.1.1 evidence and v0.1.3–v0.1.5 document sets were
moved to the
[Inherited Rust/Tauri Documents — Unreviewed Archive](../legacy/rust-tauri-inherited-unreviewed/README.md).

Those files are historical input only. Their feature statuses, migration
numbers, Rust services, Tauri commands, generated bindings, test counts, and
release claims do not describe this repository.

Future release design starts from the current Go code and active contracts. It
must not move an archived plan back here without revalidating and rewriting it.
