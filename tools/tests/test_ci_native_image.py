"""Guard the native CI bootstrap, coverage and publication trust boundary."""
import importlib.util
from pathlib import Path
import re
import os
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


class NativeImageWorkflowTests(unittest.TestCase):
    def test_bootstrap_or_immutable_digest_is_shared_by_all_six_jobs(self):
        workflow = (ROOT / '.github/workflows/check.yml').read_text()
        image = (ROOT / '.github/ci-native-image.txt').read_text().strip()
        self.assertTrue(image == '' or re.fullmatch(
            r'ghcr.io/mnemosynelab/nestworth-ci@sha256:[0-9a-f]{64}', image))
        self.assertIn('container: &native-ci\n      image: ${{ needs.runtime.outputs.image }}', workflow)
        self.assertEqual(workflow.count('container: *native-ci'), 2)
        self.assertEqual(workflow.count('needs: runtime'), 3)
        self.assertIn('python3 tools/ci-native-container.py >> "$GITHUB_OUTPUT"', workflow)
        self.assertIn('defaults:\n  run:\n    shell: bash', workflow)
        self.assertEqual(workflow.count('if: ${{ !job.container.id }}'), 6)
        self.assertEqual(workflow.count('bash tools/ci-install-native-deps.sh'), 3)
        self.assertEqual(workflow.count('bash build/ci/verify-native.sh --with-toolchains'), 3)
        # Keep action-managed caches, all test shards and native checks intact.
        self.assertEqual(workflow.count('cache-dependency-path: |\n            go.sum\n            .github/ci-native-image.txt'), 3)
        self.assertIn('cache: pnpm', workflow)
        self.assertIn('NESTWORTH_WAILS_GOCACHE=$(go env GOCACHE)', workflow)
        for gate in ('go vet ./...', 'go build ./cmd/nestworth', 'go test -count=1 -json',
                     'pnpm run generate:bindings', 'pnpm run check:bindings',
                     'pnpm run lint', 'pnpm run typecheck', 'pnpm run build',
                     'pnpm exec vitest run --shard=', 'needs: [meta, frontend, go-shard]'):
            self.assertIn(gate, workflow)
        frontend = workflow.split('  frontend:', 1)[1].split('  go-shard:', 1)[0]
        self.assertLess(frontend.index('actions/setup-node@v7'), frontend.index('pnpm/action-setup@v6'))
        self.assertLess(frontend.index('pnpm/action-setup@v6'), frontend.index('cache: pnpm'))
        meta = workflow.split('  frontend:', 1)[0]
        self.assertIn('actions/setup-node@v7', meta)
        self.assertIn('node-version: 24.22.0', meta)
        self.assertIn('node --version\n', meta)
        self.assertLess(meta.index('node --version'), meta.index('python3 -m unittest'))

    def test_cgo_cache_is_pinned_and_modules_are_shared(self):
        workflow = (ROOT / '.github/workflows/check.yml').read_text()
        # setup-go v7 hashes every listed dependency file into an exact key.
        # No restore prefix may cross the native-image boundary.
        self.assertEqual(workflow.count('id: go'), 3)
        self.assertEqual(workflow.count("if: steps.go.outputs.cache-hit != 'true'"), 3)
        self.assertEqual(workflow.count('path: ${{ env.GOMODCACHE }}'), 3)
        module_key = "key: go-modules-v1-${{ runner.os }}-${{ runner.arch }}-${{ hashFiles('go.sum') }}"
        self.assertEqual(workflow.count(module_key), 3)
        self.assertIn('GOMODCACHE: /tmp/nestworth-ci-go-modules', workflow)
        self.assertNotIn('restore-keys:', workflow)
        self.assertNotIn('go clean -modcache', workflow)
        self.assertEqual(workflow.count('name: Reset unpinned native build cache\n        if: ${{ !job.container.id }}\n        run: go clean -cache'), 3)
        # Cleanup must follow apt and precede any native compilation, bindings,
        # or verification, in every job definition (including both matrices).
        for job, end in (('meta', 'frontend'), ('frontend', 'go-shard'), ('go-shard', 'test')):
            section = workflow.split('\n  ' + job + ':', 1)[1].split('\n  ' + end + ':', 1)[0]
            self.assertLess(section.index('bash tools/ci-install-native-deps.sh'), section.index('go clean -cache'))
            self.assertLess(section.index('go clean -cache'), section.index('bash build/ci/verify-native.sh'))

    def test_runtime_bootstrap_and_digest_validation(self):
        spec = importlib.util.spec_from_file_location('ci_native_container', ROOT / 'tools/ci-native-container.py')
        runtime = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(runtime)
        self.assertEqual(runtime.image_for(''), '')
        image = 'ghcr.io/mnemosynelab/nestworth-ci@sha256:' + 'a' * 64
        self.assertEqual(runtime.image_for(image), image)
        for invalid in ('ubuntu:26.04', 'ghcr.io/mnemosynelab/nestworth-ci:latest',
                        image[:-1], image + '\ncontainer=bad', image.replace('nestworth-ci', 'other')):
            with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                runtime.image_for(invalid)

    def test_private_credentials_are_job_scoped_not_runtime_outputs(self):
        workflow = (ROOT / '.github/workflows/check.yml').read_text()
        self.assertEqual(workflow.count('packages: read'), 3)
        self.assertNotIn('packages: write', workflow)
        self.assertIn('credentials:\n        username: ${{ github.actor }}\n        password: ${{ secrets.GITHUB_TOKEN }}', workflow)
        self.assertEqual(workflow.count('secrets.'), 1)
        self.assertIn('options: --user 1001:1001', workflow)
        runtime = workflow.split('\n  runtime:', 1)[1].split('\n  meta:', 1)[0]
        self.assertNotIn('secrets.', runtime)
        self.assertNotIn('packages:', runtime)
        self.assertIn('image: ${{ steps.image.outputs.image }}', runtime)
        self.assertNotIn('pull_request_target', workflow)
        # One native environment per reviewed pin on every event; do not add a
        # fork shortcut that drops gates or mixes host/image compiled caches.
        self.assertNotIn('github.event.pull_request.head.repo', workflow)
        with tempfile.TemporaryDirectory() as directory:
            pin = Path(directory) / 'pin'
            token = 'synthetic-token-must-not-enter-outputs'
            for image in ('', 'ghcr.io/mnemosynelab/nestworth-ci@sha256:' + 'b' * 64):
                pin.write_text(image + '\n')
                result = subprocess.run([sys.executable, str(ROOT / 'tools/ci-native-container.py'), str(pin)],
                                        env={**os.environ, 'GITHUB_TOKEN': token}, text=True,
                                        capture_output=True, check=True)
                self.assertEqual(result.stdout, 'image=' + image + '\n')
                self.assertNotIn(token, result.stdout + result.stderr)

    def test_context_contains_only_explicit_public_build_inputs(self):
        context = ROOT / 'build/ci'
        self.assertEqual((context / '.dockerignore').read_text().splitlines(),
                         ['*', '!Dockerfile', '!packages.txt', '!verify-native.sh'])
        dockerfile = (context / 'Dockerfile').read_text()
        self.assertRegex(dockerfile, r'FROM ubuntu:26\.04@sha256:[0-9a-f]{64}')
        copies = [line for line in dockerfile.splitlines() if line.startswith('COPY ')]
        self.assertEqual(len(copies), 2)
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
        self.assertIn('nestworth-ghcr-verify', publish)
        self.assertIn('docker pull --platform linux/amd64 "$digest"', publish)
        self.assertIn('docker run --rm --platform linux/amd64 --user 1001:1001', publish)
        self.assertNotIn('gh api', publish)


if __name__ == '__main__':
    unittest.main()
