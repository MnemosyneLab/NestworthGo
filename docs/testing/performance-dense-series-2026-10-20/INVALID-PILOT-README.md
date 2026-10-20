# Invalid exploratory runs

The folders `current-main-paired`, `current-main-paired-final`, and
`current-main-paired-final2` contain preserved partial raw logs. In those
attempts the baseline test binary was mistakenly compiled from the candidate
worktree, so both arms used candidate production code. Do not use these files
for a baseline comparison. The valid source-verified data is in
`current-main-paired-final3`.

The partial raw files are retained for source/command provenance.
