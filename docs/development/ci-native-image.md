# Linux native CI image

The six `check.yml` work jobs (meta, two frontend shards, three Go shards)
share one runtime container definition, including one immutable private GHCR digest after
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
script. The Dockerfile copies those two public tooling files only. No application source,
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

## Private registry access and data boundary

Each of the six consumers has `contents: read` and `packages: read`; runtime
selection and the aggregate retain the workflow's `contents: read` default.
The image is pulled using `container.credentials` with `github.actor` and the
job's ephemeral `secrets.GITHUB_TOKEN`, before any checkout or run step.
A step-level `docker login` would happen too late for a job container.
Repository Actions access to the package is a prerequisite; no PAT or new
persistent ACL grant is created by these workflows.

The runtime resolver emits only `image=<validated pin>`. Credentials are
referenced directly in job YAML, never in JSON, job outputs, cache keys,
artifacts or image layers. The image field is resolved from the runtime job's
output; a blank pin keeps the hosted-runner apt/bootstrap path. A constant
empty image is rejected by GitHub's workflow validator, and the secrets context
is only available under the nested credentials field, so keep these fields
separate. See [context availability](https://docs.github.com/en/actions/reference/workflows-and-actions/contexts#context-availability)
and [container registry credentials](https://docs.github.com/en/actions/how-tos/write-workflows/choose-where-workflows-run/run-jobs-in-a-container#defining-credentials-for-a-container-registry).
Runtime failure skips the workers and the existing `test` aggregate fails
closed. A configured image that cannot be pulled also fails visibly; it never
silently skips native tests or changes environments for a subset of events.

For fork and Dependabot PRs, custom repository secrets are unavailable but
`GITHUB_TOKEN` remains available with read-only permissions. The consumers
request only package read, without changing the `pull_request` event or using
`pull_request_target`. GitHub documents that forks of a public repository may
access private packages granted to that repository. Treat this image as
readable by untrusted PR workloads: **public build tools and public native
dependencies only**. No application source, ledger, settings, credentials,
private dependency content or runner filesystem belongs in it. Private package
visibility is not a confidentiality guarantee for this tooling image.
See [fork PR token restrictions](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#workflows-in-forked-repositories)
and [package workflow access](https://docs.github.com/en/packages/learn-github-packages/configuring-a-packages-access-control-and-visibility#ensuring-workflow-access-to-your-package).

An actual digest must be tested on main/manual, same-repository PR, fork PR
and Dependabot runs before claiming those paths work. A blank-pin run validates
bootstrap and syntax, not private pull authorization. If an event cannot pull,
keep the failure visible and review the access/fallback design; do not add a
PAT, raise persistent access or weaken a gate. Rolling every worker back to a
blank pin remains available and keeps the documented host-cache cleanup.

## Trusted publishing and first-use bootstrap

`ci-native-image.yml` builds and smoke-tests relevant PR changes with
`contents: read`, without registry credentials or publication. Publication
runs only for relevant pushes to `main` or an explicit dispatch on `main`.
That job alone has `packages: write` and uses its short-lived `GITHUB_TOKEN`.
Checkout credentials are not persisted. There is no PAT, repository write,
release publication, automatic CI activation, package visibility edit, or
persistent permission configuration. New tags include source SHA, run ID
and attempt; they are candidate identifiers, never CI's runtime reference.
The image's OCI source label links the tooling package to this repository.

After push succeeds, the publisher retains the real digest and separately
requires an authenticated `linux/amd64` pull and native smoke with the same
job's ephemeral token. Its Docker authentication directories are outside all
Go/pnpm cache paths and are removed by EXIT traps. The package remains private;
anonymous access is not the acceptance test. Layer uploads or an untagged
attestation do not establish a published runnable image. Preserve provenance
and check the normal platform manifest/index and actual execution.

The initial `.github/ci-native-image.txt` stays blank until a real runnable
candidate has been verified. Activation is a separate reviewed edit:

1. Merge the credential/bootstrap workflow through normal review. This task
   does not merge it. A relevant main push or an explicit main dispatch builds
   and publishes using only that workflow's token. Registry denial must be
   diagnosed; it does not authorize credential or permission escalation.
2. Obtain `ghcr.io/mnemosynelab/nestworth-ci@sha256:<64 hex>` from a successful
   push, with authenticated platform pull and native smoke evidence. Never
   substitute an attestation digest, image ID, tag, guessed digest or uploaded
   layer hash. The retained candidate artifact alone is insufficient if the
   pull/smoke step failed.
3. Put that real digest in `.github/ci-native-image.txt`. All six workers use
   the same image and credentials definition. Run the complete `check` workflow
   with cold and restored Go/pnpm caches; verify apt is skipped, bindings CLI
   tests execute, and setup/post-cache steps succeed. Exercise the event types
   described above and keep any unverified case explicit.
4. Merge activation only after the applicable gates pass. Keep package ACLs,
   organization package policy and visibility unchanged. No public-package
   permission is required for authenticated private consumption.

Ordinary application PRs neither build nor publish the dependency image.
When dependencies/base inputs change, keep CI on the prior working digest
while a candidate builds, validate the candidate, then update that one digest.
Do not automatically follow tags or switch credentials after a denial.

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
