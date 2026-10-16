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


## Local test application identity and data isolation

All local testing, debug builds and native macOS QA must use the established
`build/darwin/Info.dev.plist` identity: bundle identifier `com.nestworth.dev`,
application name and display name `Nestworth` (bundle `Nestworth.app`). Do not
create task-specific QA names, random bundle identifiers or additional testing
identities. Production packaging retains its separate release identity.

Isolate each test run through explicit synthetic database/settings directories,
not a new app identity. Before launch, verify the bundle's dev identity, resolve
`NESTWORTH_DATABASE_PATH` and `NESTWORTH_SETTINGS_PATH` to the intended isolated
run directory, and verify a synthetic test marker/household in that database.
Use a launcher/configuration that preserves these paths when opened by native UI
tools; an app launch without verified overrides is prohibited. Never use or
modify a real household ledger for tests. Do not change system security settings
or automatically remove previously created QA applications.

If an accepted authorization flow stalls or repeats, stop requesting the same
permission. Preserve completed work and logs, mark blocked checks accurately,
and proceed with available verification and the authorized commit workflow.
