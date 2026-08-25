# Cloud Agent base image for the Nestworth Go + Fyne desktop app.
# Stable system dependencies (Go toolchain, Fyne/OpenGL/X11/Wayland build deps,
# and a headless X stack for running the GUI) live here. Repository-specific
# work (module download, build) lives in the `install` command instead.
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8

# Base tooling plus Fyne desktop prerequisites and a headless display stack:
# - build-essential/pkg-config/git/curl: Go cgo builds and VCS
# - libgl1-mesa-dev, xorg-dev, libwayland-dev, wayland-protocols,
#   libxkbcommon-dev: GLFW/Fyne X11 + Wayland backends
# - libgl1-mesa-dri: software OpenGL rendering under Xvfb
# - xvfb, x11-utils, openbox, xdotool: run and drive the GUI headlessly
# - sqlite3: inspect the local SQLite business database during development
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
      git \
      build-essential \
      pkg-config \
      libgl1-mesa-dev \
      xorg-dev \
      libwayland-dev \
      wayland-protocols \
      libxkbcommon-dev \
      libgl1-mesa-dri \
      xvfb \
      x11-utils \
      openbox \
      xdotool \
      sqlite3 \
 && rm -rf /var/lib/apt/lists/*

# Go toolchain pinned to the version required by go.mod (go 1.26.0).
ARG GO_VERSION=1.26.0
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tgz \
 && tar -C /usr/local -xzf /tmp/go.tgz \
 && rm /tmp/go.tgz \
 && ln -sf /usr/local/go/bin/go /usr/local/bin/go \
 && ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

ENV PATH=/usr/local/go/bin:/usr/local/bin:$PATH
ENV GOTOOLCHAIN=local

USER ubuntu
