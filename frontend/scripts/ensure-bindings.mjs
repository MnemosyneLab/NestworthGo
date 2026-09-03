import { existsSync, mkdtempSync, readdirSync, readFileSync, rmSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { homedir, tmpdir } from "node:os";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const repoRoot = join(frontendRoot, "..");
const bindingsRoot = join(frontendRoot, "bindings");
const marker = join(
	bindingsRoot,
	"github.com/waltwang/nestworth-go/internal/wailsapi/app/index.ts",
);
const checkOnly = process.argv.includes("--check");

const env = {
	...process.env,
	PATH: `${join(homedir(), "go/bin")}:${process.env.PATH ?? ""}`,
	GOCACHE: process.env.NESTWORTH_WAILS_GOCACHE ?? "/tmp/nestworth-wails-gocache",
};

function run(command, args) {
	return spawnSync(command, args, { cwd: repoRoot, env, stdio: "inherit" });
}

function generate(outputDirectory) {
	const args = ["generate", "bindings", "-clean=true", "-ts", "-i", "-d", outputDirectory, "./..."];
	let result = run("wails3", args);
	if (result.error?.code === "ENOENT") {
		result = run("go", ["run", "github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.16", ...args]);
	}
	return result;
}

function filesUnder(root) {
	const files = [];
	const visit = (directory) => {
		for (const entry of readdirSync(directory, { withFileTypes: true })) {
			const path = join(directory, entry.name);
			if (entry.isDirectory()) visit(path);
			else files.push(relative(root, path));
		}
	};
	if (existsSync(root)) visit(root);
	return files.sort();
}

function assertEqualBindings(expectedRoot, actualRoot) {
	const expected = filesUnder(expectedRoot);
	const actual = filesUnder(actualRoot);
	if (expected.length !== actual.length || expected.some((path, index) => path !== actual[index])) {
		throw new Error("generated Wails bindings differ in file set");
	}
	for (const path of expected) {
		if (!readFileSync(join(expectedRoot, path)).equals(readFileSync(join(actualRoot, path)))) {
			throw new Error(`generated Wails binding is stale: ${path}`);
		}
	}
}

if (checkOnly) {
	if (!existsSync(marker)) {
		console.error("Wails bindings are missing; run pnpm run generate:bindings.");
		process.exit(1);
	}
	const temporaryRoot = mkdtempSync(join(tmpdir(), "nestworth-wails-bindings-"));
	try {
		console.log("Checking generated Wails bindings against frontend/bindings...");
		const result = generate(temporaryRoot);
		if (result.status !== 0) process.exit(result.status ?? 1);
		assertEqualBindings(temporaryRoot, bindingsRoot);
		console.log("Generated Wails bindings are current.");
	} catch (error) {
		console.error(error instanceof Error ? error.message : error);
		process.exit(1);
	} finally {
		rmSync(temporaryRoot, { recursive: true, force: true });
	}
} else {
	console.log("Generating Wails TypeScript bindings...");
	const result = generate(bindingsRoot);
	if (result.status !== 0) {
		console.error("Failed to generate frontend/bindings. Install Wails v3.0.0-beta.16 and retry from the repository root:");
		console.error("  wails3 generate bindings -ts -i -d frontend/bindings ./...");
		process.exit(result.status ?? 1);
	}
	if (!existsSync(marker)) {
		console.error("Wails reported success, but the expected binding files were not written.");
		process.exit(1);
	}
}
