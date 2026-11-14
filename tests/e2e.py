#!/usr/bin/env python3
"""Exercise the built binary through a real PTY; only mutate a temporary tree."""
import fcntl
import os
from pathlib import Path
import pty
import select
import shlex
import shutil
import struct
import subprocess
import sys
import tempfile
import termios
import time

BIN = Path(sys.argv[1] if len(sys.argv) > 1 else "bin/try-go").resolve()


def interact(env, args, keys, expected=0, forbidden=None):
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 100, 0, 0))
    def session():
        os.setsid()
        fcntl.ioctl(0, termios.TIOCSCTTY, 0)
    proc = subprocess.Popen([str(BIN), "exec", *args], stdin=slave,
                            stderr=slave, stdout=subprocess.PIPE, env=env,
                            preexec_fn=session)
    os.close(slave)
    transcript = b""
    try:
        deadline = time.monotonic() + 8
        while b"Search >" not in transcript:
            if time.monotonic() > deadline:
                raise AssertionError("TUI did not render: " + repr(transcript[-2000:]))
            if select.select([master], [], [], .1)[0]:
                transcript += os.read(master, 65536)
        for key in keys:
            os.write(master, key)
            # Let the input parser distinguish Esc from the next independent key.
            until = time.monotonic() + .12
            while time.monotonic() < until:
                if select.select([master], [], [], .02)[0]:
                    try:
                        transcript += os.read(master, 65536)
                    except OSError:
                        break
        while proc.poll() is None:
            if time.monotonic() > deadline:
                raise AssertionError("TUI did not exit: " + repr(transcript[-3000:]))
            if select.select([master], [], [], .05)[0]:
                try:
                    transcript += os.read(master, 65536)
                except OSError:
                    break
        stdout, _ = proc.communicate(timeout=2)
        assert proc.returncode == expected, (proc.returncode, transcript[-3000:])
        if forbidden:
            assert forbidden not in transcript, "filesystem name injected terminal control sequence"
        if expected == 0:
            assert stdout.startswith(b"# if you can read this"), stdout
            assert b"\x1b" not in stdout, "TUI escaped onto stdout"
            assert b"\x1b[?1049l" in transcript, "alternate screen not restored"
        else:
            assert not stdout, stdout
        return stdout
    finally:
        if proc.poll() is None:
            proc.kill()
            proc.wait()
        os.close(master)


def main():
    with tempfile.TemporaryDirectory(prefix="try-go-cold-", dir=os.environ.get("TMPDIR")) as tmp:
        cold = Path(tmp) / "not-created"
        env = os.environ | {"TRY_PATH": str(cold), "TERM": "xterm-256color", "TRY_DATE": "", "HERDR_ENV": "0", "CMUX_SOCKET_PATH": "", "CMUX_BUNDLE_ID": ""}
        interact(env, ["cold-start"], [b"\x1b"], expected=1)
        assert not cold.exists(), "cancel created workspace root"
        interact(env, ["cold-start"], [b"\r"])
        assert (cold / "cold-start").is_dir()
    with tempfile.TemporaryDirectory(prefix="try-go-e2e-", dir=os.environ.get("TMPDIR")) as tmp:
        root = Path(tmp)
        base = root / "incubator"
        base.mkdir()
        projects = root / "Ray"
        projects.mkdir()
        clones = root / "clones"
        clones.mkdir()
        env = os.environ | {"TRY_PATH": str(base), "TRY_PROJECTS": str(projects),
                            "TRY_CLONE_PATH": str(clones), "TRY_DATE": "",
                            "TERM": "xterm-256color", "NO_COLOR": "1",
                            "HERDR_ENV": "0", "CMUX_SOCKET_PATH": "", "CMUX_BUNDLE_ID": ""}
        interact(env, ["demo"], [b"\r"])
        assert (base / "demo").is_dir()
        interact(env, ["demo"], [b"\r"])
        interact(env, ["demo"], [b"\x12", b"\x01\x0brenamed", b"\r"])
        assert (base / "renamed").is_dir() and not (base / "demo").exists()
        interact(env, ["renamed"], [b"\x07", b"\r"])
        assert (base / "renamed").is_symlink() and (projects / "renamed").is_dir()
        interact(env, ["renamed"], [b"\x04", b"\r", b"YES", b"\r"])
        assert not (base / "renamed").exists() and (projects / "renamed").is_dir()
        interact(env, [], [b"\x14", "中文 idea".encode(), b"\r"])
        assert (base / "中文-idea").is_dir()
        interact(env, [], [b"\x1b"], expected=1)
        # Insertion with Ctrl-T is available even when the filter matches existing names.
        interact(env, ["中文"], [b"\x14", b"\r"])
        assert (base / "中文").is_dir()
        # Shell wrappers actually change their parent shell's working directory.
        for shell in ("bash", "zsh", "fish"):
            if not shutil.which(shell):
                continue
            snippet = subprocess.check_output([str(BIN), "init", shell], env=env, text=True)
            code = snippet + "\ntry new shell-" + shell + "\npwd\n"
            p = subprocess.run([shell, "-c", code], env=env, capture_output=True, text=True, timeout=5)
            assert p.returncode == 0, (shell, p.stderr)
            assert p.stdout.strip() == str(base / ("shell-" + shell)), (shell, p.stdout)
        hostile_entry = base / "workspace\x1b]52;c;dHJ5\x07"
        hostile_entry.mkdir()
        interact(env, ["workspace"], [b"\x04", b"\r", b"\x1b", b"\x1b", b"\x1b"], expected=1, forbidden=b"\x1b]52;")
        assert hostile_entry.is_dir(), "cancel changed hostile entry"
        # Quoting must preserve fish backslashes, apostrophes, and executable paths too.
        for shell in ("bash", "zsh", "fish"):
            if not shutil.which(shell):
                continue
            hostile_root = root / (shell + r'''x\'";touch PWNED;#''')
            hostile_root.mkdir()
            hostile_bin = root / (shell + r'''bin\'";touch BIN_PWNED;#''')
            shutil.copy2(BIN, hostile_bin)
            special_env = env | {"TRY_PATH": str(hostile_root)}
            snippet = subprocess.check_output([str(hostile_bin), "init", shell], env=special_env, text=True)
            p = subprocess.run([shell, "-c", snippet + "\ntry new through-function\npwd\n"], env=special_env, cwd=root, capture_output=True, text=True, timeout=5)
            assert not (root / "PWNED").exists() and not (root / "BIN_PWNED").exists(), (shell, "command injection")
            assert p.returncode == 0 and p.stdout.strip() == str(hostile_root / "through-function"), (shell, p.stdout, p.stderr)
        print("PASS: PTY cold-root/create/select/rename/graduate/delete/unicode/cancel/Ctrl-T + bash/zsh/fish cd and hostile-path quoting")


if __name__ == "__main__":
    main()
