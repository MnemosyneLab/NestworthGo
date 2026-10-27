# Linux native CI image

The six `check.yml` work jobs (meta, two frontend shards, three Go shards)
share one container definition, including one immutable GHCR digest after
activation. All native tests, CGO builds, vet, Wails bindings generation and
comparison, frontend lint/typecheck/build/tests, and the required `test`
aggregate remain in place. Go and pnpm dependencies still use `setup-go`,
`setup-node` and their existing caches. This image is independent of the
server/distroless image in `build/docker/Dockerfile.server`.

## Inputs and provenance

`build/ci/Dockerfile` pins the official `ubuntu:26.04` base by digest, matching
the current `ubuntu-26.04` runner generation. It installs only public packages
from the base's signed official Ubuntu `resolute`, updates, backports and
security archives; it does not inherit the runner's Azure mirror settings.
`build/ci/packages.txt` is the explicit top-level package manifest. It includes
GTK4, WebKitGTK 6, libsoup 3, GCC/G++/make/libc headers via build-essential,
pkg-config, GNU coreutils, Bash, git, CA certificates, Python 3, archive,
process and text utilities. Transitive packages and exact installed versions
are recorded in `/usr/local/share/nestworth-ci/installed-packages.tsv`, beside
`requested-packages.txt` and `ubuntu.sources`.

Ubuntu 26.04's build-essential depends on the uutils provider. The image keeps
that distro dependency intact, installs `gnu-coreutils`, and shadows only
`timeout` with `/usr/local/bin/timeout -> /usr/bin/gnutimeout`. The native
runtime guard requires GNU timeout and exercises directory permissions.
Rebuilds refresh public package versions; an activated image never changes
under its digest. There is no automatic rebuild schedule: dependency changes
or an explicit main dispatch produce candidates. Security/package refreshes
require a deliberate dispatch or a reviewed input change.

Always build with the **`build/ci` context**, never `.`. Its deny-by-default
`.dockerignore` permits only the Dockerfile, manifest and native verification
script. The Dockerfile copies those two public tooling files only. No source,
ledger, home directory, runner filesystem, build output, token or credential
is copied. Docker's predefined proxy build arguments may be forwarded from a
builder environment; they are not declared as Dockerfile ARG/ENV or saved in
the image. No local proxy CA certificate is copied.

```bash
bash tools/ci-build-native-image.sh nestworth-ci:local
```

The helper builds without layer reuse, verifies native pkg-config entries and
GNU timeout, then smoke-tests Actions-like bind mounts. Image `USER` and the
workflow `--user` both select `1001:1001`, matching hosted Ubuntu's runner UID.
The image grants no passwordless sudo. The verification script rejects root,
proves that a mode-0500 directory cannot be written, and, in check jobs,
requires writable home, checkout, runner temp/tool cache, Go build/module
cache and enabled CGO. This protects existing permission assertions such as
`TestMetadataDeletionFailureDoesNotBreakDisable`; no assertion is weakened.
All run steps explicitly use Bash, including arrays/mapfile/process
substitution. Meta explicitly sets up Node 24.22.0 and runs `node --version`
before Python discovery, preventing `BindingsCLITests` from silently skipping
in a minimal image. Frontend bootstraps Node/npm before `pnpm/action-setup`
(which invokes npm), then restores its existing setup-node pnpm cache once
pnpm is available; both setup-node steps reuse the same tool installation.
Bindings query `go env GOCACHE` explicitly, since a minimal container need
not export the hosted runner's `GOCACHE` environment variable.
Frontend retains Node 24.22.0 and pnpm 12.14.0; Go comes
from `go.mod` (currently 1.27.3), never from a separate image toolchain pin.

## Trusted publishing and first-use bootstrap

`ci-native-image.yml` builds and smoke-tests relevant PR changes with
`contents: read`, without registry credentials or publication. Publication
runs only for relevant pushes to `main` or an explicit dispatch on `main`.
That job alone has `packages: write` and uses its short-lived `GITHUB_TOKEN`.
Checkout credentials are not persisted. There is no PAT, repository write,
release publication, automatic CI activation, package visibility edit, or
persistent permission configuration. New tags include source SHA, run ID
and attempt; they are candidate identifiers, never CI's runtime reference.
The image's OCI source label links the public tooling to this repository.

The initial container anchor intentionally has `image: ${{ '' }}`. GitHub's
runner treats an empty evaluated container image as no job container; the
existing bounded apt helper then runs on the hosted runner. Consequently,
merging bootstrap does not introduce a nonexistent image dependency or skip
any gate. The shared image must stay empty until a real candidate exists.
See the runner's
[container conversion](https://github.com/actions/runner/blob/main/src/Sdk/DTPipelines/Pipelines/ObjectTemplating/PipelineTemplateConverter.cs)
and GitHub's [container shell documentation](https://docs.github.com/en/actions/how-tos/write-workflows/choose-where-workflows-run/run-jobs-in-a-container).

Activation is a separate reviewed edit after these prerequisites:

1. Merge the bootstrap workflow through normal review. This task does not
   merge it. The first relevant main push builds/publishes a candidate using
   only that workflow's token, or an owner dispatches it explicitly on main.
2. Obtain its exact `ghcr.io/mnemosynelab/nestworth-ci@sha256:<64 hex>` from
   the workflow artifact/summary. A first GHCR package normally defaults to
   private. An owner must separately authorize and perform any package
   visibility/access change; the workflow never does so. The publish job's
   anonymous pull gate will fail if public access is not ready. See GitHub's
   [GHCR authentication and visibility guidance](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry).
3. With a fresh, unauthenticated Docker configuration, pull that digest and
   smoke-test it as UID 1001. If the first publication failed only at public
   access, repeat this verification after owner action. Never replace the
   empty anchor with a guessed digest, tag or private-only image. Public
   anonymous access is required for fork PRs without registry secrets.
4. Set only `jobs.meta.container`'s anchored `image` to the real full digest.
   All six jobs inherit it. Run the whole `check` workflow on that activation
   commit, including cold and restored Go/pnpm caches, verify no apt step ran,
   all bindings CLI tests executed, and all setup/post-cache steps succeeded.
   Merge activation only after these gates pass. A missing/private/broken
   configured image must fail visibly, not silently change test environments.

Ordinary application PRs neither build nor publish the dependency image.
When dependencies/base inputs change, keep CI on the prior working digest
while a candidate builds, verify the candidate with the same activation
procedure, then update that one digest. Do not automatically follow tags.

## Rollback and elapsed-time evidence

Restore the last known-good digest to roll back an image update. Set the
shared image back to `${{ '' }}` to return all six jobs to the original apt
helper. That path is slower and remains subject to mirror availability, but
retains its existing bounded retry/diagnostic behavior. The apt helper is
preserved for `perf.yml` and bootstrap; check containers never run it.

Before claiming a speedup, compare complete `check` runs on equivalent source
and runner versions, with cold and warm Go/pnpm cache states recorded. Include
queue, image pull/container setup, native setup, build, test and cache/post-job
time; report run wall clock and the slowest job. Six concurrent apt durations
must not be added together as a wall-clock saving. Image size, registry pull
speed and native compilation can change the result. Local image smoke/build
measurements alone do not establish whole-workflow gains or macOS behavior.
