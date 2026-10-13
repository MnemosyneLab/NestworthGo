# Frontend changes

Follow the [shared-control contract](../docs/development/frontend-controls.md)
and the existing engineering guide. Inspect `src/components/ui`,
`src/components/layout`, relevant feature wrappers, and their tests before
introducing a control. Extend the canonical API when necessary instead of
copying its implementation or using a platform-specific fallback.

Run lint, typecheck, frontend tests and production build. Lint enforces control
boundaries; regression tests cover interaction. Tests may use native-input
stubs to inject invalid values, but changed controls also need real-component
interaction coverage. Record browser and native checks separately.
