# Inherited Rust/Tauri Documents — Unreviewed Archive

## Status

Everything under this directory was inherited from the former Nestworth
Rust/Tauri/React implementation line and has not been corrected or validated
for Nestworth-go unless a future review explicitly says otherwise.

These files are retained only as historical product/design input. They are not:

- current Go + Fyne implementation evidence;
- executable migration, schema, package, or API contracts;
- proof that a feature, phase, test, build, or release exists in this repository;
- safe implementation plans to execute without a full redesign.

Current truth remains the code, migrations, and tests in Nestworth-go, followed
by the active documents linked from [`docs/README.md`](../../README.md).

## Contents

The [`releases`](releases/) directory contains:

- the original mixed v0.1.1 contract, including inherited Rust/Tauri phase and
  release evidence that was removed from the active Go contract;
- the inherited v0.1.3–v0.1.5 product contracts;
- their Rust/Tauri technical designs and implementation plans;
- their old compatibility baselines where present.

The original inherited [product roadmap](product-roadmap.md) is also preserved
here. The active roadmap has been rewritten to distinguish verified Go work,
approved v0.1.2 design, and unreviewed future direction.

v0.1.2 is not archived here because it has been rewritten and revalidated as
the active Go + Fyne design for the next release.

## Revalidation Rule

Reusing an archived product decision requires checking it against the current
Go domain, SQLite schema number, application ports, Fyne ownership, tests, and
earlier active release contracts. Reusing an implementation detail requires a
new Go/Fyne technical design and implementation plan in `docs/releases/`.

Do not move an archived file back into the active release directory merely by
changing its status banner.
