# syntax=docker/dockerfile:1
# CI tooling only. Never use the repository root as the build context.
FROM ubuntu:26.04@sha256:19fcd1bf268f02b60aca3bd30bd8a6d9a11759a430e32921833e65f16d87348a AS native

LABEL org.opencontainers.image.source="https://github.com/MnemosyneLab/NestworthGo" \
      org.opencontainers.image.description="Nestworth Linux native CI dependencies; no application or ledger data"

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]
COPY packages.txt /usr/local/share/nestworth-ci/requested-packages.txt
# Use the official Ubuntu archives and signed Ubuntu metadata, not the
# runner's Azure mirror. Package versions and sources are retained below.
# Only image builds contact apt; PR test containers never install packages.
RUN apt-get -o Acquire::Retries=3 -o Acquire::http::Timeout=30 \
      -o APT::Update::Error-Mode=any update \
    && mapfile -t packages < /usr/local/share/nestworth-ci/requested-packages.txt \
    && DEBIAN_FRONTEND=noninteractive apt-get -o Acquire::Retries=3 \
      -o Acquire::http::Timeout=30 install -y --no-install-recommends \
      "${packages[@]}" \
    # Keep the distro provider (build-essential requires uutils), but put GNU
    # timeout first on PATH for both production helpers and signal tests.
    && ln -s /usr/bin/gnutimeout /usr/local/bin/timeout \
    && timeout --version | grep -q 'GNU coreutils' \
    && dpkg-query -W -f='${binary:Package}\t${Version}\n' \
      > /usr/local/share/nestworth-ci/installed-packages.tsv \
    && cp /etc/apt/sources.list.d/ubuntu.sources /usr/local/share/nestworth-ci/ubuntu.sources \
    && rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/* \
    && groupadd --gid 1001 runner \
    && useradd --uid 1001 --gid 1001 --create-home --shell /bin/bash runner

COPY --chmod=755 verify-native.sh /usr/local/bin/nestworth-ci-verify-native
# Match the GitHub hosted Linux runner UID. No passwordless sudo grant.
USER 1001:1001
ENV LANG=C.UTF-8
WORKDIR /home/runner
RUN nestworth-ci-verify-native

# No application context enters either variant. Go/Wails versions are supplied
# by ci-toolchain-versions.py from go.mod, not maintained a second time here.
FROM native AS toolchain-build
USER 0:0
ARG GO_VERSION
ARG GO_SHA256
ARG WAILS_VERSION
RUN --mount=type=secret,id=proxy_ca \
    test -n "$GO_VERSION" && test -n "$WAILS_VERSION" \
    && [[ "$GO_SHA256" =~ ^[0-9a-f]{64}$ ]] \
    && ca_args=() \
    && if [[ -f /run/secrets/proxy_ca ]]; then \
         ca_args=(--cacert /run/secrets/proxy_ca); export SSL_CERT_FILE=/run/secrets/proxy_ca; \
       fi \
    && curl --fail --location --retry 3 "${ca_args[@]}" \
         "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tar.gz \
    && echo "$GO_SHA256  /tmp/go.tar.gz" | sha256sum --check --strict \
    && mkdir -p /opt/nestworth-toolchain/bin \
    && tar -xzf /tmp/go.tar.gz -C /opt/nestworth-toolchain \
    && export PATH="/opt/nestworth-toolchain/go/bin:$PATH" GOTOOLCHAIN=local \
    && test "$(go version)" = "go version go${GO_VERSION} linux/amd64" \
    && GOBIN=/opt/nestworth-toolchain/bin GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod \
         go install "github.com/wailsapp/wails/v3/cmd/wails3@${WAILS_VERSION}" \
    && test "$(/opt/nestworth-toolchain/bin/wails3 version 2>&1)" = "$WAILS_VERSION" \
    && printf 'Go=%s\nWails=%s\nGoArchiveSHA256=%s\n' "$GO_VERSION" "$WAILS_VERSION" "$GO_SHA256" \
         > /opt/nestworth-toolchain/versions.txt \
    && chmod -R a+rX /opt/nestworth-toolchain

FROM native AS toolchain
ARG GO_VERSION
ARG WAILS_VERSION
LABEL io.nestworth.ci.go="$GO_VERSION" io.nestworth.ci.wails="$WAILS_VERSION"
COPY --from=toolchain-build /opt/nestworth-toolchain /opt/nestworth-toolchain
ENV PATH="/opt/nestworth-toolchain/go/bin:/opt/nestworth-toolchain/bin:${PATH}" \
    GOTOOLCHAIN=local
# /__t is mounted over by Actions; tools must remain outside its tool cache.
USER 1001:1001
RUN test "$(/opt/nestworth-toolchain/go/bin/go version)" = "go version go${GO_VERSION} linux/amd64" \
    && test "$(/opt/nestworth-toolchain/bin/wails3 version 2>&1)" = "$WAILS_VERSION" \
    && nestworth-ci-verify-native
