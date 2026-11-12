"""Guard the native CI bootstrap, coverage and publication trust boundary."""
import importlib.util
from pathlib import Path
import re
import subprocess
import tempfile
import unittest
from datetime import datetime, timedelta, timezone

ROOT = Path(__file__).resolve().parents[2]


class NativeImageWorkflowTests(unittest.TestCase):
    def test_native_jobs_share_the_reviewed_image_and_node_consumers_use_verified_bindings(self):
        workflow = (ROOT / '.github/workflow-archive/check-linux.yml.disabled').read_text()
        image = (ROOT / '.github/ci-native-image.txt').read_text().strip()
        self.assertTrue(image == '' or re.fullmatch(
            r'ghcr.io/mnemosynelab/nestworth-ci@sha256:[0-9a-f]{64}', image))
        self.assertIn('container: &native-ci ${{ fromJSON(needs.runtime.outputs.container) }}', workflow)
        self.assertEqual(workflow.count('container: *native-ci'), 2)
        self.assertEqual(workflow.count('needs: runtime'), 3)
        self.assertIn('python3 tools/ci-native-container.py >> "$GITHUB_OUTPUT"', workflow)
        self.assertIn('defaults:\n  run:\n    shell: bash', workflow)
        self.assertEqual(workflow.count('if: ${{ !job.container.id }}'), 6)
        self.assertEqual(workflow.count('bash tools/ci-install-native-deps.sh'), 3)
        self.assertEqual(workflow.count('bash build/ci/verify-native.sh --with-toolchains'), 3)
        # Keep action-managed caches, all test shards and native checks intact.
        self.assertEqual(workflow.count('run: bash tools/ci-use-toolchain.sh activate-go'), 3)
        self.assertEqual(workflow.count("if: needs.runtime.outputs.preinstalled != 'true'"), 3)
        self.assertEqual(workflow.count('          cache: false\n'), 3)
        self.assertIn('cache: pnpm', workflow)
        self.assertIn('NESTWORTH_WAILS_GOCACHE=%s', workflow)
        for gate in ('go vet ./...', 'go build ./cmd/nestworth', 'go test -count=1 -json',
                     'node scripts/ensure-bindings.mjs',
                     'pnpm exec eslint .', 'pnpm exec tsc --noEmit', 'pnpm exec vite build --mode production',
                     'pnpm exec vitest run --shard=', 'needs: [meta, frontend-bindings, frontend-quality, frontend, go-shard]'):
            self.assertIn(gate, workflow)
        frontend = workflow.split('  frontend-bindings:', 1)[1].split('  frontend-quality:', 1)[0]
        self.assertIn('actions/setup-node@v7', frontend)
        self.assertNotIn('pnpm/action-setup', frontend)
        self.assertNotIn('pnpm install', frontend)
        self.assertNotIn('node scripts/ensure-bindings.mjs --check', frontend)
        self.assertNotIn('bindings-compare', frontend)
        self.assertIn("if: steps.bindings-restore.outputs.valid != 'true'", frontend)
        self.assertIn('python3 tools/bindings-cache.py restore', frontend)
        self.assertIn('python3 tools/bindings-cache.py save', frontend)
        self.assertIn('key=disabled', frontend)
        self.assertIn('^[0-9a-f]{64}$', frontend)
        self.assertIn('continue-on-error: true', frontend)
        # Only compiled Go artifacts may use the fully fenced snapshot prefix.
        self.assertEqual(frontend.count('restore-keys:'), 1)
        self.assertLess(frontend.index("run: node scripts/ensure-bindings.mjs\n"), frontend.index("- name: Share verified bindings"))
        meta = workflow.split('  frontend-bindings:', 1)[0]
        self.assertIn('actions/setup-node@v7', meta)
        self.assertIn('node-version: 24.22.3', meta)
        self.assertIn('node --version\n', meta)
        self.assertLess(meta.index('node --version'), meta.index('python3 tools/run-tooling-tests.py'))
        for job, end in (("frontend-quality", "frontend"), ("frontend", "go-shard")):
            consumer = workflow.split("\n  " + job + ":", 1)[1].split("\n  " + end + ":", 1)[0]
            self.assertIn("needs: frontend-bindings", consumer)
            self.assertIn("actions/download-artifact@v8", consumer)
            self.assertIn("name: frontend-bindings-${{ github.sha }}", consumer)
            self.assertIn("path: frontend/bindings", consumer)
            self.assertLess(consumer.index("actions/setup-node@v7"), consumer.index("pnpm/action-setup@v6"))
            self.assertLess(consumer.index("pnpm/action-setup@v6"), consumer.index("cache: pnpm"))
            self.assertNotIn("actions/setup-go", consumer)
            self.assertNotIn("container:", consumer)
            self.assertNotIn("ensure-bindings.mjs", consumer)
        self.assertIn("if-no-files-found: error", frontend)
        self.assertIn("overwrite: true", frontend)
        cli = frontend.split("- name: Cache pinned Wails CLI binary", 1)[1].split("- name: Generate Wails bindings", 1)[0]
        self.assertIn("if: ${{ job.container.id && needs.runtime.outputs.preinstalled != 'true' }}", frontend.split("- name: Cache pinned Wails CLI binary", 1)[1].split("- name: Prepare pinned Wails CLI", 1)[0])
        self.assertIn("path: /tmp/nestworth-ci-wails", cli)
        self.assertNotIn("GOMODCACHE", cli)
        self.assertIn("${{ steps.go.outputs.go-version }}", cli)
        self.assertIn("hashFiles('go.mod', '.github/ci-native-image.txt', '.github/ci-toolchain-image.txt')", cli)
        self.assertIn('run: bash tools/ci-use-toolchain.sh wails', cli)
        self.assertNotIn('go install', cli)
        self.assertIn('PREINSTALLED: ${{ needs.runtime.outputs.preinstalled }}', cli)
        self.assertLess(frontend.index("go clean -cache"), frontend.index("- name: Cache pinned Wails CLI binary"))
        self.assertLess(frontend.index("- name: Prepare pinned Wails CLI"), frontend.index("node scripts/ensure-bindings.mjs"))

    def test_cgo_cache_is_pinned_and_modules_are_shared(self):
        workflow = (ROOT / '.github/workflow-archive/check-linux.yml.disabled').read_text()
        # Independent module/build caches replace setup-go's combined cache.
        # No restore prefix may cross the native-image boundary.
        self.assertEqual(workflow.count('        id: go\n'), 3)
        self.assertEqual(workflow.count('path: ${{ env.GOCACHE }}'), 3)
        self.assertEqual(workflow.count('id: go-build-cache'), 3)
        self.assertEqual(workflow.count('restore-keys:'), 3)
        self.assertIn('cache-day: ${{ steps.image.outputs.cache-day }}', workflow)
        self.assertNotIn('go-build-v2-', workflow)
        self.assertEqual(workflow.count("if: needs.runtime.outputs.preinstalled == 'true'"), 3)
        self.assertEqual(workflow.count('path: ${{ env.GOMODCACHE }}'), 3)
        module_key = "key: go-modules-v1-${{ runner.os }}-${{ runner.arch }}-${{ hashFiles('go.sum') }}"
        self.assertEqual(workflow.count(module_key), 3)
        self.assertIn('GOMODCACHE: /tmp/nestworth-ci-go-modules', workflow)
        # Prefix fallback is limited to the exact Go, native/dependency fence
        # and owner. Neither another shard nor the old shared archive can match.
        for job, end, owner in (('meta', 'frontend-bindings', 'meta'),
                                ('frontend-bindings', 'frontend-quality', 'bindings'),
                                ('go-shard', 'test', '${{ matrix.artifact }}')):
            section = workflow.split('\n  ' + job + ':', 1)[1].split('\n  ' + end + ':', 1)[0]
            cache = section.split('- name: Cache image-specific Go build artifacts', 1)[1].split('- name: Install native', 1)[0]
            prefix = "go-build-v3-${{ runner.os }}-${{ runner.arch }}-${{ steps.go.outputs.go-version }}-${{ hashFiles('go.mod', 'go.sum', '.github/ci-native-image.txt', '.github/ci-toolchain-image.txt') }}-" + owner + '-'
            self.assertIn('key: ' + prefix + '${{ needs.runtime.outputs.cache-day }}', cache)
            self.assertIn('restore-keys: |\n            ' + prefix + '\n', cache)
            restore_lines = []
            for line in cache.split('restore-keys: |', 1)[1].splitlines()[1:]:
                if not line.strip():
                    continue
                if not line.startswith('            '):
                    break
                restore_lines.append(line.strip())
            self.assertEqual(restore_lines, [prefix])
            self.assertEqual(cache.count('go-build-v3-'), 2)
            self.assertNotIn('github.sha', cache)
            self.assertNotIn('github.run_id', cache)
            self.assertIn('if: ${{ job.container.id }}', cache)
        self.assertNotIn('actions: write', workflow)
        matrix = workflow.split('\n  go-shard:', 1)[1].split('    steps:', 1)[0]
        owners = ['meta', 'bindings'] + re.findall(r'^            artifact: (.+)$', matrix, re.M)
        self.assertEqual(len(owners), 6)
        self.assertEqual(len(owners), len(set(owners)))
        self.assertNotIn('go clean -modcache', workflow)
        self.assertEqual(workflow.count('name: Reset unpinned native build cache\n        if: ${{ !job.container.id }}\n        run: go clean -cache'), 3)
        # Cleanup must follow apt and precede any native compilation, bindings,
        # or verification, in every job definition (including both matrices).
        for job, end in (('meta', 'frontend-bindings'), ('frontend-bindings', 'frontend-quality'), ('go-shard', 'test')):
            section = workflow.split('\n  ' + job + ':', 1)[1].split('\n  ' + end + ':', 1)[0]
            self.assertLess(section.index('bash tools/ci-install-native-deps.sh'), section.index('go clean -cache'))
            self.assertLess(section.index('go clean -cache'), section.index('bash build/ci/verify-native.sh'))

    def test_snapshot_day_is_shared_utc_and_refreshes_after_midnight(self):
        spec = importlib.util.spec_from_file_location('ci_native_snapshot', ROOT / 'tools/ci-native-container.py')
        runtime = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(runtime)
        morning = datetime(2026, 11, 2, 1, tzinfo=timezone.utc)
        late = morning.replace(hour=23, minute=59)
        self.assertEqual(runtime.snapshot_day(morning), runtime.snapshot_day(late))
        self.assertNotEqual(runtime.snapshot_day(late), runtime.snapshot_day(late + timedelta(minutes=1)))
        self.assertEqual(runtime.snapshot_day(datetime(2026, 11, 3, 1, tzinfo=timezone(timedelta(hours=8)))), '20261102')
        with self.assertRaises(ValueError):
            runtime.snapshot_day(datetime(2026, 11, 2))

    def test_runtime_bootstrap_and_digest_validation(self):
        spec = importlib.util.spec_from_file_location('ci_native_container', ROOT / 'tools/ci-native-container.py')
        runtime = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(runtime)
        self.assertEqual(runtime.container_for(''), '')
        image = 'ghcr.io/mnemosynelab/nestworth-ci@sha256:' + 'a' * 64
        self.assertEqual(runtime.container_for(image), {'image': image, 'options': '--user 1001:1001'})
        for invalid in ('ubuntu:26.04', 'ghcr.io/mnemosynelab/nestworth-ci:latest',
                        image[:-1], image + '\ncontainer=bad', image.replace('nestworth-ci', 'other')):
            with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                runtime.container_for(invalid)

    def test_toolchain_pin_precedence_and_blank_pin_rollback(self):
        with tempfile.TemporaryDirectory() as directory:
            native = Path(directory) / 'ci-native-image.txt'
            toolchain = native.with_name('ci-toolchain-image.txt')
            image = 'ghcr.io/mnemosynelab/nestworth-ci@sha256:'
            for native_digest, toolchain_digest, expected, installed in (
                ('a' * 64, 'b' * 64, 'b' * 64, True),
                ('a' * 64, '', 'a' * 64, False),
                ('', '', '', False),
            ):
                native.write_text(image + native_digest if native_digest else '')
                toolchain.write_text(image + toolchain_digest if toolchain_digest else '')
                result = subprocess.run(['python3', str(ROOT / 'tools/ci-native-container.py'), str(native)],
                                        capture_output=True, text=True, check=True)
                self.assertIn('preinstalled=' + str(installed).lower(), result.stdout)
                self.assertIn(image + expected if expected else 'container=""', result.stdout)
            toolchain.write_text('ghcr.io/mnemosynelab/nestworth-ci:latest')
            result = subprocess.run(['python3', str(ROOT / 'tools/ci-native-container.py'), str(native)],
                                    capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(result.stdout, '')

    def test_context_contains_only_explicit_public_build_inputs(self):
        context = ROOT / 'build/ci'
        self.assertEqual((context / '.dockerignore').read_text().splitlines(),
                         ['*', '!Dockerfile', '!packages.txt', '!verify-native.sh'])
        dockerfile = (context / 'Dockerfile').read_text()
        self.assertRegex(dockerfile, r'FROM ubuntu:26\.04@sha256:[0-9a-f]{64}')
        copies = [line for line in dockerfile.splitlines() if line.startswith('COPY ')]
        self.assertEqual(len(copies), 3)
        self.assertEqual(copies[2], 'COPY --from=toolchain-build /opt/nestworth-toolchain /opt/nestworth-toolchain')
        self.assertIn('COPY packages.txt ', copies[0])
        self.assertIn('COPY --chmod=755 verify-native.sh ', copies[1])
        self.assertIn('USER 1001:1001', dockerfile)
        helper = (ROOT / 'tools/ci-build-native-image.sh').read_text()
        self.assertIn('"$image" build/ci', helper)
        self.assertNotIn('docker login', helper)
        self.assertNotIn('docker push', helper)

    def test_publication_is_main_only_and_uses_workflow_token(self):
        workflow = (ROOT / '.github/workflows/ci-native-image.yml').read_text()
        validate, publish = workflow.split('  publish:', 1)
        self.assertIn("if: github.event_name == 'pull_request'", validate)
        self.assertNotIn('packages: write', validate)
        self.assertNotIn('secrets.', validate)
        self.assertIn("github.ref == 'refs/heads/main'", publish)
        self.assertIn("github.event_name == 'push' || github.event_name == 'workflow_dispatch'", publish)
        self.assertIn('packages: write', publish)
        self.assertIn('secrets.GITHUB_TOKEN', publish)
        self.assertNotIn('pull_request_target', workflow)
        self.assertNotIn('contents: write', workflow)
        self.assertIn('nestworth-ghcr-anonymous', publish)
        self.assertIn('docker pull "$digest"', publish)
        self.assertNotIn('gh api', publish)


if __name__ == '__main__':
    unittest.main()
