# Temporary GitHub Actions CI Pause

**Status:** Active as of 2026-10-19. **Owner:** Engineering.

This note records the temporary pause requested while performance optimization
is in progress. The repository's only registered Actions workflow is changed
in this branch to manual-only triggering, and GitHub has also manually disabled
the workflow while older pull request branches are reviewed:

| Workflow | ID | Path | Previous state | Current state |
| --- | ---: | --- | --- | --- |
| `check` | `348385332` | `.github/workflows/check.yml` | `active` | `disabled_manually` |

The proposed workflow source on PR #120 has `workflow_dispatch` only.

The prior triggers were `push` to `main` and `pull_request` with GitHub's
default activity types (including opened, synchronize, and reopened). The
proposed change removes those automatic triggers while preserving every job
and the rest of the workflow. The server-side `disabled_manually` state remains
in effect, so no manual or automatic run can start until GitHub re-enables the
workflow. This is a GitHub Actions workflow pause only; it does not disable
external CI services or GitHub App checks.

No job or test was removed. Only the workflow trigger declaration is changed
in the proposed PR; branch protection and repository Actions settings were not
changed. If branch protection requires the `check` status, pull request merges
may remain blocked until checks are available again.

## Safe rollout and restoration

Do not re-enable the workflow automatically. Re-enabling the server-side
workflow only permits triggers declared by the workflow files; it does not
change those triggers. The `push` filter in the old configuration matched
pushes to `main`, while `pull_request` applied to pull requests targeting the
repository, including updates to existing PRs.

Before permitting even manual runs, first merge this manual-only workflow
change to the default branch while the workflow remains disabled. Then inspect
the workflow file on the default branch and every open PR head that could
produce or merge workflow changes. At the time this procedure was prepared,
the heads of PRs #117, #118, and #119 still carried the old automatic triggers;
the #120 branch contains the manual-only change. Update any remaining old
branches to the manual-only version or close them, and ensure their eventual
merges cannot restore the automatic trigger block. Recheck the complete open
PR list and workflow inventory immediately before enabling. Do not infer that
merging a change to `main` updates the workflow file on older PR branches.

Once the default branch is manual-only and no active PR head or pending merge
can reintroduce the old triggers, the user may explicitly ask to permit manual
execution. Then enable the workflow:

```bash
gh workflow enable check.yml --repo MnemosyneLab/NestworthGo
```

With `workflow_dispatch` still in the default-branch workflow, this enables
manual dispatch only. It does not restore automatic CI. Do not dispatch a run
unless separately requested. Verify the workflow state with:

```bash
gh workflow list --repo MnemosyneLab/NestworthGo --all
```

To restore automatic CI later, require the user's explicit instruction, repeat
the branch and workflow inventory review above, and make a separate reviewed
change restoring the original trigger configuration:

```yaml
on:
  push:
    branches: [main]
  pull_request:
```

Keep the workflow server-disabled until the reviewed trigger change is on the
default branch and older open PR heads and pending merges have been checked for
stale trigger definitions. After the user's instruction, re-enable it and
verify that `check` is `active`:

```bash
gh workflow enable check.yml --repo MnemosyneLab/NestworthGo
```

Verify the resulting state with:

```bash
gh workflow list --repo MnemosyneLab/NestworthGo --all
```

The expected state after enabling is `active`. This procedure does not enable
any external checks or change branch protection.
