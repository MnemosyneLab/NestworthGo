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
    def cancelled(signum, frame):
        # The command is in a separate session; killing only this fixture's
        # group would orphan it. Forward TERM and reap before the outer CI
        # scheduler's longer grace expires. Ignore repeat cancellation here.
        for sig in (signal.SIGTERM, signal.SIGINT):
            signal.signal(sig, signal.SIG_IGN)
        try:
            os.killpg(process.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        grace = min(1.0, float(os.environ.get('NESTWORTH_CI_CANCEL_GRACE', '2')) / 2)
        try:
            process.wait(timeout=grace)
        except subprocess.TimeoutExpired:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait()
        raise SystemExit(128 + signum)

    # Do not leave a spawn-to-handler window with an unowned detached child.
    signals = {signal.SIGTERM, signal.SIGINT}
    old_mask = signal.pthread_sigmask(signal.SIG_BLOCK, signals)
    process = subprocess.Popen(args.command, start_new_session=True,
                               preexec_fn=lambda: signal.pthread_sigmask(signal.SIG_SETMASK, old_mask))
    for sig in signals:
        signal.signal(sig, cancelled)
    signal.pthread_sigmask(signal.SIG_SETMASK, old_mask)
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
