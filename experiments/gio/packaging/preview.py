"""Build and open the local Gio testing application on its native platform."""

import argparse
import ctypes
import fcntl
import os
from pathlib import Path
import plistlib
import shutil
import subprocess
import sys
import tempfile

APP_ID = "com.nestworth.gio"
NAME = "Nestworth Gio"
GIO = Path(__file__).resolve().parents[1]
REPO = GIO.parents[1]
MAC = sys.platform == "darwin"


def binary(package):
    directory = "Contents/MacOS" if MAC else "bin"
    return package / directory / NAME


def verify_package(package):
    executable = binary(package)
    if package.is_symlink() or not package.is_dir():
        raise ValueError(f"Expected a Gio package directory at {package}")
    if executable.resolve() != executable or not executable.is_file():
        raise ValueError(f"Expected a regular Gio executable at {executable}")
    if MAC:
        plist = package / "Contents/Info.plist"
        if plist.is_symlink() or not plist.is_file():
            raise ValueError(f"Expected the Gio Info.plist at {plist}")
        with plist.open("rb") as source:
            info = plistlib.load(source)
        identity = {"CFBundleName": NAME, "CFBundleDisplayName": NAME,
                    "CFBundleExecutable": NAME, "CFBundleIdentifier": APP_ID}
        if any(info.get(key) != value for key, value in identity.items()):
            raise ValueError(f"Different application identity at {package}")
        subprocess.run(["codesign", "--verify", "--strict", package], check=True)
    metadata = subprocess.check_output(["go", "version", "-m", executable], text=True)
    if "\tpath\tgithub.com/waltwang/nestworth-go/experiments/gio" not in metadata.splitlines():
        raise ValueError(f"Different Go application at {executable}")


def publish(staged, target):
    # Exchange two complete packages in one rename so Finder and direct
    # launches always see an intact build, even during an update.
    if not os.path.lexists(target):
        staged.rename(target)
        return
    verify_package(target)
    libc = ctypes.CDLL(None, use_errno=True)
    src, dst = os.fsencode(staged), os.fsencode(target)
    if MAC:
        result = libc.renamex_np(src, dst, 2)  # RENAME_SWAP
    else:
        result = libc.renameat2(-100, src, -100, dst, 2)  # AT_FDCWD, RENAME_EXCHANGE
    if result != 0:
        errno = ctypes.get_errno()
        raise OSError(errno, os.strerror(errno), target)


def build():
    arch = subprocess.check_output(["go", "env", "GOHOSTARCH"], text=True).strip()
    parts = ["bin", "gio"] if MAC else ["bin", "gio", "linux-" + arch]
    output = REPO
    for part in parts:
        output /= part
        if output.is_symlink():
            raise ValueError(f"Refusing a linked build directory: {output}")
        output.mkdir(exist_ok=True)
    name = NAME + ".app" if MAC else NAME
    target = output / name
    env = dict(os.environ, CGO_ENABLED="1", GOOS=sys.platform, GOARCH=arch)
    with tempfile.TemporaryDirectory(prefix=".build-", dir=output) as scratch:
        staged = Path(scratch) / name
        executable = binary(staged)
        executable.parent.mkdir(parents=True)
        resources = executable.parents[1] / "Resources"
        resources.mkdir()
        shutil.copyfile(REPO / "frontend/src/assets/brand/wordmark.png", resources / "wordmark.png")
        shutil.copyfile(REPO / "assets/brand/favicon.png", resources / "mark.png")
        for notice in ("LICENSE", "NOTICE.md"):
            shutil.copyfile(REPO / "frontend/public/bank-logos" / notice, resources / f"bank-logos-{notice}")
        subprocess.run(["go", "build", "-mod=readonly", "-trimpath", "-ldflags",
                        "-X gioui.org/app.ID=" + APP_ID, "-o", executable, "."],
                       cwd=GIO, env=env, check=True)
        if MAC:
            shutil.copyfile(GIO / "packaging/Info.plist", staged / "Contents/Info.plist")
            shutil.copyfile(REPO / "assets/icons/icon.icns", resources / "icon.icns")
            subprocess.run(["codesign", "--force", "--sign", "-", staged], check=True)
        verify_package(staged)
        directory = os.open(output, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            fcntl.flock(directory, fcntl.LOCK_EX)
            publish(staged, target)
        finally:
            os.close(directory)
    return target


def launch(package):
    executable = binary(package)
    # The executable owns startup checks and shows a window if startup is blocked.
    if not MAC:
        os.execl(executable, NAME)
    overrides = []
    for name in ("NESTWORTH_DATABASE_PATH", "NESTWORTH_SETTINGS_PATH"):
        if value := os.environ.get(name):
            overrides.extend(["--env", f"{name}={value}"])
    if overrides:
        # An explicitly selected database may differ from an already open window.
        subprocess.run(["open", "-n", "-a", package, *overrides], check=True)
    else:
        subprocess.run(["open", package], check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--build-only", action="store_true", help="build without opening the app or its data")
    args = parser.parse_args()
    package = build()
    print(f"Built {package}", flush=True)
    if not args.build_only:
        launch(package)


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
