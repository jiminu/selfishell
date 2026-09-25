"""Compare the built native foundation to the fixed Bash reference, literally."""

import errno
import os
from pathlib import Path
import pty
import shutil
import subprocess
import sys
import tempfile

from snapshot import snapshot

REFERENCE = "3bbbfa0346ee74eb47f31a81ec666340a5ef6018"
repo = Path(__file__).resolve().parents[3]
candidate = Path(sys.argv[1]).resolve(strict=True)


def capture(executable, args, env, cwd, terminal=False):
    if not terminal:
        result = subprocess.run([str(executable), *args], env=env, cwd=cwd,
                                input=b"", capture_output=True, timeout=10)
        return result.returncode, result.stdout, result.stderr
    master, slave = pty.openpty()
    try:
        result = subprocess.run([str(executable), *args], env=env, cwd=cwd,
                                stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                stderr=slave, timeout=10)
        # Darwin may discard unread PTY output when its last slave closes.
        os.set_blocking(master, False)
        data = b""
        while True:
            try:
                part = os.read(master, 4096)
            except OSError as error:
                if error.errno in (errno.EIO, errno.EAGAIN):
                    break
                raise
            if not part:
                break
            data += part
        return result.returncode, result.stdout, data
    finally:
        os.close(master)
        if slave is not None:
            os.close(slave)


with tempfile.TemporaryDirectory(prefix="selfishell-go-foundation.") as tmp:
    root = Path(tmp)
    release = root / "release with spaces"
    release.mkdir()
    archive = root / "reference.tar"
    with archive.open("wb") as output:
        subprocess.run(["git", "-C", str(repo), "archive", REFERENCE],
                       stdout=output, check=True)
    subprocess.run(["tar", "-xf", str(archive), "-C", str(release)], check=True)
    entry = release / "bin/selfishell"
    reference = entry.read_bytes()
    home = root / "home"
    home.mkdir()
    env = {"HOME": str(home), "PATH": "/usr/bin:/bin:/usr/sbin:/sbin",
           "LC_ALL": "C", "SELFISHELL_ROOT": "/wrong/root"}
    before = snapshot(home)
    (release / ".git").touch()
    link = root / "direct"
    link.symlink_to(entry)
    chained = root / "sfs"
    chained.symlink_to("direct")
    commands = [[], [""], ["help"], ["--help"], ["-h"], ["help", ""],
                ["help", "extra"], ["unknown"], [" unknown "], ["version"],
                ["--version"], ["-v"], ["version", ""], ["version", "", "extra"],
                ["version", "extra"], ["version", "help", "extra"],
                ["version", "--help"], ["version", "-h"],
                ["version", "--available", "extra"]]
    count = 0
    for args in commands:
        entry.write_bytes(reference)
        expected = capture(chained, args, env, home)
        shutil.copyfile(candidate, entry)
        actual = capture(chained, args, env, home)
        assert actual == expected, (args, actual, expected)
        count += 1
    (release / ".git").unlink()
    for version in (b"1.2.3\n", b"1.2.3\n\n", b" v1 \r\n", b"", None):
        version_file = release / "VERSION"
        if version is None:
            version_file.unlink()
        else:
            version_file.write_bytes(version)
        entry.write_bytes(reference)
        expected = capture(entry, ["version"], env, home)
        shutil.copyfile(candidate, entry)
        actual = capture(entry, ["version"], env, home)
        assert actual == expected, (version, actual, expected)
        count += 1
    for no_color in ("", "1"):
        env["NO_COLOR"] = no_color
        entry.write_bytes(reference)
        expected = capture(entry, ["unknown"], env, home, terminal=True)
        shutil.copyfile(candidate, entry)
        actual = capture(entry, ["unknown"], env, home, terminal=True)
        assert expected[2] and (b"\x1b[31m" in expected[2]) == (not no_color)
        assert actual == expected, ("terminal", no_color, actual, expected)
        count += 1
    for command in ("status", "doctor", "update", "rollback"):
        code, out, error = capture(entry, [command], env, home)
        assert code == 1 and not out and b"not implemented in the Go candidate" in error
    for command in ("install", "uninstall"):
        assert capture(entry, [command, "--help"], env, home)[0] == 0
    assert snapshot(home) == before, "foundation commands modified HOME"
    # A generated VERSION beside bin/ is sufficient: neither Go nor the source
    # checkout is required by an installed executable.
    (release / "VERSION").write_text("0.0.0-test\n")
    env["PATH"] = str(root / "no-tools")
    assert capture(chained, ["version"], env, home) == (0, b"selfishell 0.0.0-test\n", b"")
    assert capture(chained, ["help"], env, home)[0] == 0
    print(f"Go foundation: {count} exact reference comparisons, incomplete-command and toolchain-free smoke checks passed")
