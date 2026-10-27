"""Check CLI selection with synthetic tools; never generate real bindings."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "frontend/scripts/ensure-bindings.mjs"
NODE = shutil.which("node")


@unittest.skipUnless(NODE, "Node.js is required for the bindings guard")
class BindingsCLITests(unittest.TestCase):
    def test_only_exact_stable_cli_is_used(self):
        for version, selected in (("v3.0.0", "wails3"),
                                  ("v3.0.0-beta.34", "go"),
                                  ("v3.0.0-nightly.20261027", "go"),
                                  ("v3.0.1", "go"), ("missing", "go")):
            with self.subTest(version=version), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                script = root / "frontend/scripts/ensure-bindings.mjs"
                script.parent.mkdir(parents=True)
                shutil.copyfile(SCRIPT, script)
                binaries = root / "bin"
                binaries.mkdir()
                stub = f"#!{sys.executable}\n" + '''import json, os, sys
from pathlib import Path
if sys.argv[1:] == ["version"]:
    if os.environ["TEST_CLI_VERSION"] == "missing":
        sys.exit(127)
    print(os.environ["TEST_CLI_VERSION"], file=sys.stderr)
    sys.exit(0)
Path(os.environ["TEST_CLI_LOG"]).write_text(json.dumps(sys.argv))
output = Path(sys.argv[sys.argv.index("-d") + 1])
marker = output / "github.com/waltwang/nestworth-go/internal/wailsapi/app/index.ts"
marker.parent.mkdir(parents=True)
marker.write_text("// synthetic binding")
'''
                for name in ("go", "wails3"):
                    command = binaries / name
                    command.write_text(stub)
                    command.chmod(0o755)
                log = root / "invocation.json"
                env = {**os.environ, "HOME": str(root), "PATH": str(binaries),
                       "TEST_CLI_VERSION": version, "TEST_CLI_LOG": str(log)}
                result = subprocess.run([NODE, str(script)], env=env,
                                        capture_output=True, text=True, timeout=15)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                invocation = json.loads(log.read_text())
                self.assertEqual(Path(invocation[0]).name, selected)
                if selected == "go":
                    self.assertEqual(invocation[1:3], ["run", "github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0"])
                self.assertIn("./...", invocation)


if __name__ == "__main__":
    unittest.main()
