# Temporary GitHub Actions CI Pause

**Status:** Active as of 2026-10-19. **Owner:** Engineering.

This note records the temporary pause requested while performance optimization
is in progress. GitHub has manually disabled the repository's sole registered
Actions workflow:

| Workflow | ID | Path | Previous state | Current state |
| --- | ---: | --- | --- | --- |
| `check` | `348385332` | `.github/workflows/check.yml` | `active` | `disabled_manually` |

The workflow source still declares `push` to `main` and `pull_request` triggers.
While it is disabled, it does not start for new pull requests, new commits to
pull requests, or pushes to `main` after merges. Its automated test, race,
build, lint, and check jobs are all paused. This is a GitHub Actions workflow
pause only; it does not disable external CI services or GitHub App checks.

No workflow, code, or test source was removed or edited to pause CI. Branch
protection and repository Actions settings were not changed. If branch
protection requires the `check` status, pull request merges may remain blocked
until checks are available again.

## Restoration

Do not restore CI automatically. Re-enable the workflow only after the user
explicitly asks to restore it. Before restoring, review source and configuration
changes made since the pause in other pull requests and on `main`, especially
workflow, test, and build configuration. Include the open CI sharding work in
[PR #117](https://github.com/MnemosyneLab/NestworthGo/pull/117) and
[PR #118](https://github.com/MnemosyneLab/NestworthGo/pull/118) in that review.
Confirm that the current workflow inventory and intended checks are still
appropriate; do not assume the workflow source or status is unchanged.

After that review and the user's instruction, restore the original workflow
with:

```bash
gh workflow enable check.yml --repo MnemosyneLab/NestworthGo
```

Verify the resulting state with:

```bash
gh workflow list --repo MnemosyneLab/NestworthGo --all
```

The expected state for `check` after restoration is `active`. This procedure
does not enable any external checks or change branch protection.
