# Linux native CI image

The six `check.yml` work jobs (meta, two frontend shards, three Go shards)
share one runtime container definition, including one immutable GHCR digest after
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

## CGO build-cache boundaries

`setup-go@v7` does not derive a container identity from `/etc/os-release`.
Its Linux key includes the runner's `ImageOS`, Go version and the hash of
`cache-dependency-path`; see its
[restore implementation](https://github.com/actions/setup-go/blob/v7/src/cache-restore.ts).
All six workers list **both `go.sum` and `.github/ci-native-image.txt`** in
that input. Setup-go restores only its exact key, without a broad restore
prefix. Consequently a first digest, a different digest, or a changed Go
version/dependency lock cannot restore another image's compiled CGO artifacts.
This also invalidates the previous go.sum-only archive. Each immutable digest
has its own build-cache boundary. Returning to an earlier identical pin can
restore that digest's earlier cache, if it is still retained.

Module downloads remain reusable across these boundaries. `GOMODCACHE` is
explicitly `/tmp/nestworth-ci-go-modules` on both host and container jobs.
When setup-go misses, a separate `actions/cache` restores **only that directory**
under an OS/architecture/go.sum key that excludes the image pin. Its fixed
path also avoids a cache-version/path mismatch between host and container
homes. Successful cold runs save this shared module archive and setup-go's
image-specific combined archive; warm setup-go hits skip the extra module
cache action. No compiled build-cache directory enters the shared archive.

A blank pin runs the original apt helper and then `go clean -cache` in each
worker, before any native build/test/bindings command. Apt can change C headers
without changing the blank pin, so the host path intentionally discards restored
compiled artifacts on every run. It never calls `go clean -modcache`; downloaded
modules survive. Switching to an immutable digest can reuse the module archive
but compiles against that image on its first run. Digest A -> B starts a distinct
build cache, B -> A can reuse A, and either digest -> blank cleans the host build
cache again. Frontend bindings use the resulting `go env GOCACHE` as before.

The optional `bash tools/check-ci-native-cache.sh` probe requires Go with
CGO and a C compiler. It creates only synthetic files and cache directories,
changes an external C header, and asserts fresh-image compilation, rollback
and unpinned cleanup while retaining a module marker. With Go 1.27.3 the
shared-cache run still returned header value 1 after the header changed to 2;
the distinct cache returned 2, rollback returned 1, and cleanup returned 3.
It remains opt-in because it cold-compiles standard-library cache entries.

Blank-pin bootstrap therefore has cold native compilation and an extra cache
fallback on misses. Image activation and warm digest runs have different cache
costs; whole-workflow cold/warm timing still requires measurement.

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

The initial `.github/ci-native-image.txt` is blank. A lightweight `runtime`
job reads this reviewed pin and emits JSON for the entire container value:
empty string for hosted-runner bootstrap, or an image/options mapping for a
validated immutable digest. GitHub rejects a constant empty `container.image`
before scheduling jobs, so workers resolve the whole container from this
runtime output. No nonexistent image is referenced. The original bounded apt
helper runs only on the host path; every test gate remains required. Failure
of the runtime job skips its workers and the existing `test` aggregate fails
closed. See the runner's
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
   blank pin file with a guessed digest, tag or private-only image. Public
   anonymous access is required for fork PRs without registry secrets.
4. Put the real full digest in `.github/ci-native-image.txt`.
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
pin file back to blank to return all six jobs to the original apt
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
