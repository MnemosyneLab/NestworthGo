# Linux native CI images

`check.yml` uses a reviewed public image for meta, the bindings producer and
four Go shards. Frontend quality and six frontend test shards stay on hosted
Node; they consume the verified, exact-run bindings artifact and do not pull a
native image. All existing gates, coverage, timeouts and perf tiers remain.
This is independent of the application/server image in `build/docker`.

## Two variants and one version source

`build/ci/Dockerfile` has two runtime targets in the same GHCR package:

| Target | Contents | Intended use |
| --- | --- | --- |
| `native` | Ubuntu 26.04, GTK4/WebKitGTK6/libsoup3, C tooling and CI utilities | Existing dependency-only runtime and rollback |
| `toolchain` | The same native layers plus fixed Go and Wails CLI | Native Go jobs without cold Go download/CLI compilation |

The toolchain variant contains neither application source nor generated bindings,
Node, pnpm, module downloads or build caches. Its last layer contains only
`/opt/nestworth-toolchain`: Go, `bin/wails3`, and a version/checksum manifest.
Do not install tools under `/__t`: Actions mounts its writable tool cache there,
hiding image contents. `/opt` remains root-owned and readable/executable by UID
1001. Caches, home, workspace and runner temporary paths remain writable.

`tools/ci-toolchain-versions.py` reads Go's patch version and the Wails v3
requirement from **`go.mod`**, currently Go 1.27.3 and Wails v3.1.0. The image
Dockerfile, labels, CLI fallback and workflow cache identity derive from those
values. No separate editable image version pins exist. The parser rejects
ambiguous versions, toolchain directives and Wails replacements until explicit
support is added. The builder obtains the Go archive SHA256 from official Go
release metadata over TLS and checks the downloaded archive; Go's normal module
checksum verification applies to the public Wails module. The final image checks
both executables as UID 1001 and records their versions plus Go archive checksum.

Go/Wails version edits automatically trigger candidate build/smoke on PRs and
candidate publication after a reviewed main push, because `ci-native-image.yml`
watches `go.mod` as well as build inputs and the version parser. Other go.mod
changes conservatively rebuild too. Node is not preinstalled, so its existing
`.nvmrc`/workflow setup does not need a native-image rebuild.

Build both targets on the same builder in order to share **identical native
layers**, rather than independently rebuilding apt in parallel jobs:

```bash
bash tools/ci-build-native-image.sh nestworth-ci:local-native native
bash tools/ci-build-native-image.sh nestworth-ci:local-toolchain toolchain
```

The helper never logs in or pushes. Fresh hosted builders resolve current signed
Ubuntu packages; rebuilding locally may reuse apt layers. Use Docker's explicit
cache invalidation when deliberately refreshing packages on a reused local
builder. A fixed activated digest never changes underneath CI.

## Public build inputs and unprivileged execution

The base pins official `ubuntu:26.04` by digest. `packages.txt` selects public
packages from signed official Ubuntu archives. Requested and installed package
versions and apt sources are retained under `/usr/local/share/nestworth-ci`.
The distro's uutils dependency remains intact; `/usr/local/bin/timeout` selects
GNU `gnutimeout`. The runtime guard requires GNU timeout and proves a mode-0500
directory cannot be written. Both image USER and job options select `1001:1001`,
without passwordless sudo, preserving permission-sensitive tests.

Build **only with `build/ci` as context**, never the repository root. Its
deny-by-default `.dockerignore` admits only the Dockerfile, package manifest and
native verification script. Go/Wails versions and the public checksum are build
arguments; application files, generated output, ledgers, runner snapshots,
credentials and user profiles never enter the context or final image. Docker's
predefined proxy arguments may be forwarded but are not declared or persisted
in image configuration. If `CODEX_PROXY_CERT` is configured, the helper mounts
it as the optional BuildKit secret `proxy_ca` for networked toolchain steps;
no session CA is copied into an image layer.

The helper tests Actions-like mounted home, workspace, temp and tool-cache
permissions. `verify-native.sh --with-toolchains` additionally checks CGO and
writable module/build caches. Meta still sets up and explicitly checks Node
before tooling-test discovery, preventing silent skips of bindings CLI tests.

## Runtime selection and cache boundaries

Two reviewed pins control runtime selection:

- `.github/ci-native-image.txt` retains the existing working native digest.
- `.github/ci-toolchain-image.txt` starts blank. When nonblank, it takes
  precedence for native jobs and explicitly requires preinstalled tools.

`tools/ci-native-container.py` validates both as immutable digests in the approved
GHCR package, emits the whole container JSON and the `preinstalled` flag, and
fails on invalid pins. A blank toolchain pin selects the native pin; both blank
select hosted-runner apt bootstrap. Missing/private/broken configured images fail
visibly, rather than silently changing test environments.

`ci-use-toolchain.sh` validates `/opt/nestworth-toolchain/go/bin/go` against
go.mod before adding that explicit directory to PATH. It verifies the selected
Go again before cache restoration. `GOTOOLCHAIN=local` prevents an unnoticed Go
upgrade. On the toolchain path, `setup-go` and CLI binary cache/install are
skipped; the producer validates and selects the explicit image Wails executable.
An incorrect or missing image executable **fails closed** and requires an image
rebuild/digest review. The legacy native/host paths retain setup-go and a
version-verified Wails installation; only immutable legacy images may cache that
CLI, keyed by Go, architecture, go.mod and both reviewed pins.

Go preparation now separates the caches even when setup-go is skipped:

- Modules: `/tmp/nestworth-ci-go-modules`, OS/architecture/go.sum exact key,
  reusable across native and toolchain variants. The existing module-only key
  remains compatible. No compiled artifacts enter this archive.
- Build: `/tmp/nestworth-ci-go-build`, OS/architecture/actual Go version plus
  both digest files and go.sum, with a fresh `go-build-v2` namespace and no
  restore prefixes. Another image or Go/lock version cannot restore it.
  Existing setup-go combined archives are intentionally not reused.

The host rollback never restores/saves the image build archive. It still runs
the bounded apt helper followed by `go clean -cache` **before** native compilation
and verification; unpinned C headers cannot reactivate stale compiled artifacts.
It retains modules and never calls `go clean -modcache`. The opt-in
`tools/check-ci-native-cache.sh` synthetic header probe exercises these boundaries.

Bindings remain generated outside the image. Their conservative exact-input
fingerprint, CLI binary hash, manifest/content validation and regenerate-on-miss
fallback are unchanged. There is one generation when required; CI does not add
an automatic second generation or comparison. Pure frontend jobs receive that
same-run artifact and retain existing pnpm caches.

## Trusted publication and reviewed adoption

PRs build/smoke both variants with `contents: read` and no registry credentials.
Only relevant main pushes or an explicit main dispatch publish candidates,
using the existing job's ephemeral `GITHUB_TOKEN` and `packages: write`.
Both tags include source SHA/run/attempt and the target suffix. No PAT,
repository write, PR write, package visibility change, release or auto-merge is
added. Publication anonymously pulls/verifies each digest and retains both in
an artifact even if the public-access check fails.

Automatic rebuild/publication is deliberately separate from adoption:

1. Review and merge the image/workflow implementation normally. This task does
   not merge. Its main push publishes `native` and `toolchain` candidates.
2. Read each exact digest from the image workflow summary/artifact. Confirm
   anonymous pull, tool versions, architecture and UID/cache permissions. The
   existing public package needs no visibility change; a private candidate must
   fail instead of introducing registry credentials into PRs.
3. Put the verified toolchain digest in `.github/ci-toolchain-image.txt` through
   a separate reviewable activation PR, leaving the native fallback pin intact.
   Do not guess the digest, use `latest`, or pin a private-only image.
4. Run every `check` gate on that exact activation commit. Verify setup-go/CLI
   installation are skipped, and compare cold/warm cache and end-to-end timings.
   Merge activation only after the coordinator reviews evidence.

Future Go/Wails upgrades use the same candidate-publication/pin-update sequence.
To keep a version-upgrade PR green while the old toolchain digest is incompatible,
blank the toolchain pin in that PR and temporarily use the lean native fallback;
its updated main then publishes the replacement toolchain for separate adoption.
An automatic pin-update PR would require repository/PR write permission not
currently granted to this workflow. This change does not expand those permissions
or create credentials; the supported update mechanism is a normal reviewed pin PR.

Rollback by clearing the toolchain pin, restoring a known-good toolchain digest,
or clearing both pins for apt bootstrap. A nonblank toolchain pin takes precedence,
so changing only the native pin does not deactivate it. `perf.yml` retains its
hosted apt path and existing heavy-validation boundaries.

## Performance evaluation

Compare complete workflows on equivalent source/runner versions. Record queue,
image pull/container init, tool setup, module/build/bindings cache restore/save,
compilation/test and post-job time. Concurrent jobs' savings cannot be summed
into wall-clock savings. A warm CLI cache already avoids compilation; the
preinstalled image primarily stabilizes the cold path and removes Go setup.

The supplied baseline run `41400162960`, producer job `125130122871`, recorded
24s container init, 24s setup-go (about 10.3s tool installation plus 13s restoring
a 314MB combined cache), 6s setup-node, 66s cold CLI installation and 38s bindings
generation. A PR with a warm CLI cache used 2s restore and 0s preparation. Equal
keys do not let main read a PR-ref cache. These are baseline observations, not
measured savings from this change.

Node remains on setup-node. Only meta and the bindings producer need it among
native jobs; adding it to the common toolchain makes all four Go shards pull it.
The official Node 24.22.0 linux-x64 archive measured 32,015,600 bytes compressed
and about 204 MiB extracted in this cloud environment; even the bare executable
is about 121 MiB. A local minimum Node-only image added exactly 126,816,376
uncompressed bytes to the existing native image; gzip-6 compression of that
executable measured 44,722,946 bytes (an estimate of an incremental registry
layer, not a published digest measurement). Download/extract measured 0.38s/1.73s on this environment's
network, which does not predict hosted GHCR pull speed. Against a 6s baseline
setup-node step, there is insufficient demonstrated end-to-end gain to justify
its cost on every Go shard or a third runtime image. Pure frontend jobs should
continue using the host Node tool cache.

An expanded single image makes every native job pay all tool costs and removes
the lean rollback/consumer choice. Two layered variants keep that choice without
independent base maintenance; consumers fetch only their selected variant, and
Docker reuses identical layers when both variants are pulled on the same host.
Hosted jobs usually have isolated daemons, so cross-job layer reuse is not assumed.
Local image/build/pull evidence is supplementary; hosted CI and native macOS
results must be reported separately.

Local validation on Linux amd64 (2026-11-02, Go 1.27.3 / Wails v3.1.0):

- Both final targets built and passed UID/cache-mount smoke, including empty
  `/__t` and deliberate Go/Wails mismatches. The native image was 1,235,996,874
  bytes and toolchain 1,540,694,266 bytes, a 304,697,392-byte increment. All seven
  native layers were the exact prefix of the toolchain image's nine layers.
- Compressing a tar stream of the installed tool directory with gzip-6 measured
  104,858,345 bytes. This estimates the additional registry transfer; no new
  image was published from this environment, so it is not a GHCR layer/pull
  measurement. The old published native digest has 421,398,591 compressed
  layer bytes; its local cold/warm pull measured 76.91s/0.53s.
- Two paired local `docker run --rm ... /bin/true` samples were native/toolchain
  8.06s/12.39s and 9.21s/7.71s. This daemon uses **vfs**, and the tiny, variable
  sample establishes neither a startup improvement nor hosted-runner latency.
- With isolated synthetic caches, module download took 114.96s, native build
  58.47s cold and 0.22s warm, and the existing metadata-deletion permission
  regression passed. On the final image and source snapshot `9a2b7b5`, native
  build took 0.37s warm, the **first** bindings generation passed in 16.43s,
  manifest save (including unchanged-input validation) took 0.37s, and validated
  restore took 0.06s. Go compilation was already warm for that generation, so
  comparing 16.43s directly with the supplied 38s baseline would be misleading.
  Node 24.22.0 was mounted for this smoke, matching its separate CI setup role.

The first run after this cache migration has a cold `go-build-v2` namespace;
the legacy CLI's version-derived `wails-cli-v2` key may also need one seed before
toolchain adoption. Module-only archives remain compatible. Candidate publishing,
anonymous toolchain digest pull, activation PR checks, hosted cold/warm whole-run
timing, and macOS execution remain separate, unverified rollout steps.
