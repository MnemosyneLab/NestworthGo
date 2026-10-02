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

A follow-up Chromium check used the real Base UI `Sheet` with each institution
and account picker. From both the search field and a result button, the first
Escape closes only the picker, restores focus to its summary and preserves the
unsaved name. A second Escape closes the Sheet normally. Account-page regression
tests exercise both focus targets with the actual account edit Sheet. The open
picker stops Escape propagation; the closed picker leaves Escape untouched.
