"""POSIX timeout fixture for synthetic CI tests on macOS and Linux.

Only the timeout options used by ci-install-native-deps.sh are supported. This
is not a production replacement for GNU timeout; Linux CI still uses coreutils.
Start a separate process group so TERM/KILL reach the synthetic command tree.
"""
import argparse
import os
import signal
import subprocess
import sys


def seconds(value):
    return float(value[:-1] if value.endswith('s') else value)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--verbose', action='store_true')
    parser.add_argument('-k', type=seconds, required=True)
    parser.add_argument('budget', type=seconds)
    parser.add_argument('command', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    process = subprocess.Popen(args.command, start_new_session=True)
    try:
        status = process.wait(timeout=args.budget)
        return status if status >= 0 else 128 - status
    except subprocess.TimeoutExpired:
        if args.verbose:
            print('portable timeout fixture: sending TERM', file=sys.stderr, flush=True)
        os.killpg(process.pid, signal.SIGTERM)
        try:
            process.wait(timeout=args.k)
            return 124
        except subprocess.TimeoutExpired:
            if args.verbose:
                print('portable timeout fixture: sending KILL', file=sys.stderr, flush=True)
            os.killpg(process.pid, signal.SIGKILL)
            process.wait()
            return 137


if __name__ == '__main__':
    sys.exit(main())
