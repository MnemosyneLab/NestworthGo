# Gio desktop preview

This standalone Go module contains the Nestworth sidebar and a Gio application shell. Overview shows three sample balances. Directory contains a multiline input for testing input methods, editing, and selection. **Development → Component Preview** contains a sample form with an institution name and multiline notes. The form keeps its draft in memory.

The sidebar footer switches between Simplified Chinese and English, and between light and dark appearance. Labels use the message catalog in `localization.go` with an English fallback. Appearance colors come from `appearance.go`. Each preview launch starts in Simplified Chinese and light mode.

To check live switching, open the component preview, enter text in both fields, and select part of the notes. Switch language and appearance in both directions. The component preview stays open and keeps the draft and selection.

On Apple Silicon with the repository's Go and Xcode toolchain, run from the repository root:

```sh
sh experiments/gio/preview-macos.sh
```

The preview launcher prepares an isolated synthetic fixture and presents the development `Nestworth.app` with bundle ID `com.nestworth.dev`. It prints the run directory containing the bundle and logs.
