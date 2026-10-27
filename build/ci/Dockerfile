# CI tooling only. Never use the repository root as the build context.
FROM ubuntu:26.04@sha256:19fcd1bf268f02b60aca3bd30bd8a6d9a11759a430e32921833e65f16d87348a

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
