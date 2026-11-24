# Gio desktop preview

This standalone Go module contains the Nestworth sidebar and a Gio workspace. Overview shows three sample balances. Directory contains a multiline input for testing input methods, editing, and selection.

The sidebar footer switches between Simplified Chinese and English, and between light and dark appearance. Labels use the message catalog in `localization.go` with an English fallback. Appearance colors come from `appearance.go`. Each preview launch starts in Simplified Chinese and light mode, with preferences and editor text kept in memory for that run.

On Apple Silicon with the repository's Go and Xcode toolchain, run from the repository root:

```sh
sh experiments/gio/preview-macos.sh
```

The preview launcher prepares an isolated synthetic fixture and presents the development `Nestworth.app` with bundle ID `com.nestworth.dev`. It prints the run directory containing the bundle and logs.
