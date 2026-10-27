"""Guard the native CI bootstrap, coverage and publication trust boundary."""
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]


class NativeImageWorkflowTests(unittest.TestCase):
    def test_bootstrap_or_immutable_digest_is_shared_by_all_six_jobs(self):
        workflow = (ROOT / '.github/workflows/check.yml').read_text()
        config = re.search(r'container: &native-ci\n      image: ([^\n]+)\n', workflow)
        self.assertIsNotNone(config)
        image = config[1].strip("'\"")
        self.assertTrue(image == "${{ '' }}" or re.fullmatch(
            r'ghcr.io/mnemosynelab/nestworth-ci@sha256:[0-9a-f]{64}', image))
        self.assertEqual(workflow.count('container: *native-ci'), 2)
        self.assertIn('options: --user 1001:1001', workflow)
        self.assertIn('defaults:\n  run:\n    shell: bash', workflow)
        self.assertEqual(workflow.count('if: ${{ !job.container.id }}'), 3)
        self.assertEqual(workflow.count('bash tools/ci-install-native-deps.sh'), 3)
        self.assertEqual(workflow.count('bash build/ci/verify-native.sh --with-toolchains'), 3)
        # Keep action-managed caches, all test shards and native checks intact.
        self.assertEqual(workflow.count('cache-dependency-path: go.sum'), 3)
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
        self.assertIn('nestworth-ghcr-anonymous', publish)
        self.assertIn('docker pull "$digest"', publish)
        self.assertNotIn('gh api', publish)


if __name__ == '__main__':
    unittest.main()
