# Gio desktop preview

This standalone Go module contains the Chinese Nestworth sidebar and a Gio workspace. Overview shows three sample balances. Directory contains a multiline input for testing input methods, editing, and selection. The preview keeps all UI state in memory.

On Apple Silicon with the repository's Go and Xcode toolchain, run from the repository root:

```sh
sh experiments/gio/preview-macos.sh
```

The preview launcher prepares an isolated synthetic fixture and presents the development `Nestworth.app` with bundle ID `com.nestworth.dev`. It prints the run directory containing the bundle and logs.
