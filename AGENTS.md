# Repository guidance

Read `docs/development/engineering-guide.md` before changes and
`docs/development/local-workflow.md` for commands and platform gates. Preserve
financial, generated-artifact and isolated-data rules in those documents.
No repository `.agents/skills` is currently present; `skills/nestworth` is the
end-user ledger skill, not a development workflow.

For frontend work, read `frontend/AGENTS.md`. Discover existing components and
nearby tested call sites before creating controls. Fix recurring issues at the
owning shared component and add a practical regression or static guard. Keep
exceptions explicit; do not redesign unrelated screens or move business rules
into presentation code. Report native macOS verification separately from Linux
builds, jsdom tests and browser checks.
