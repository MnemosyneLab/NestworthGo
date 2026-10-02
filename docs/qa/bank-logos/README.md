# Bank logo component review

2026-10-02, Chromium on Linux, synthetic institution/account state using the
actual `IconPicker` and `EntityIcon` components. All 608 local SVGs decoded with
nonzero dimensions and no browser page errors. Keyboard Enter/Space selected a
logo, Escape returned focus to the trigger, and the inheritance action restored
the institution logo. No personal ledger or external financial service used.

- [Light theme](light.png)
- [Dark theme](dark.png)

The images show both source variants (compact mark and wordmark). Source
whitespace is preserved; compact marks are more legible at account-icon sizes.
This is a component preview, not native macOS or a complete Wails session.
