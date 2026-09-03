import { existsSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const repoRoot = join(frontendRoot, "..");
const marker = join(
  frontendRoot,
  "bindings/github.com/waltwang/nestworth-go/internal/wailsapi/app/index.ts",
);
const force = process.argv.includes("--force");

if (!force && existsSync(marker)) {
  process.exit(0);
}

const env = {
  ...process.env,
  PATH: `${join(homedir(), "go/bin")}:${process.env.PATH ?? ""}`,
};

console.log("Generating Wails TypeScript bindings...");

function run(command, args) {
  return spawnSync(command, args, { cwd: repoRoot, env, stdio: "inherit" });
}

let result = run("wails3", [
  "generate",
  "bindings",
  "-clean=true",
  "-ts",
  "-i",
  "./...",
]);
if (result.error?.code === "ENOENT") {
  result = run("go", [
    "run",
    "github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.16",
    "generate",
    "bindings",
    "-clean=true",
    "-ts",
    "-i",
    "./...",
  ]);
}

if (result.status !== 0) {
  console.error(
    "Failed to generate frontend/bindings. Install Wails v3.0.0-beta.16 and retry from the repository root:",
  );
  console.error("  wails3 generate bindings -ts -i ./...");
  process.exit(result.status ?? 1);
}

if (!existsSync(marker)) {
  console.error("Wails reported success, but the expected binding files were not written.");
  process.exit(1);
}
