"""PTY measurement helper for benchmark.sh; uses its disposable HOME and ZDOTDIR."""

import fcntl
import math
import os
from pathlib import Path
import pty
import select
import signal
import struct
import sys
import termios
import time


def wait_for_prompt(fd, count, timeout=15):
    # Editing may redraw the previous prompt: only a new precmd counts.
    marker = f"__SFS_READY_{count}__".encode()
    deadline = time.monotonic() + timeout
    data = b""
    while marker not in data:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise RuntimeError(f"Prompt {count} timed out: {data[-2000:]!r}")
        if select.select([fd], [], [], remaining)[0]:
            chunk = os.read(fd, 65536)
            if not chunk:
                raise RuntimeError(f"Shell exited before prompt {count}: {data[-2000:]!r}")
            data = (data + chunk)[-65536:]
    return time.perf_counter()


def finish_shell(pid, fd):
    # Let Zsh release history locks before another sample starts.
    os.write(fd, b"exit\n")
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        exited, status = os.waitpid(pid, os.WNOHANG)
        if exited:
            if status:
                raise RuntimeError(f"Shell exited with wait status {status}")
            return
        if select.select([fd], [], [], 0.05)[0]:
            try:
                os.read(fd, 65536)
            except OSError:
                pass  # Linux PTYs may report EIO while the child exits.
    raise RuntimeError("Shell did not exit after measurement")


def measure(cwd, env, iterations):
    # Keep mise from discovering personal configuration above either scenario.
    env = dict(env, MISE_CEILING_PATHS=str(Path(cwd).resolve()))
    starts, commands = [], []
    for iteration in range(iterations + 1):
        start = time.perf_counter()
        pid, fd = pty.fork()
        if pid == 0:
            try:
                fcntl.ioctl(0, termios.TIOCSWINSZ, struct.pack("HHHH", 32, 160, 0, 0))
                os.chdir(cwd)
                os.execve("/bin/zsh", ["zsh", "-d", "-i"], env)
            finally:
                os._exit(1)
        try:
            elapsed = (wait_for_prompt(fd, 1) - start) * 1000
            if iteration:
                starts.append(elapsed)
            # Fixed settling time; this does not assert deferred plugin readiness.
            time.sleep(0.1)
            for count in range(2, 5):
                start = time.perf_counter()
                os.write(fd, b":\n")
                elapsed = (wait_for_prompt(fd, count) - start) * 1000
                if iteration:
                    commands.append(elapsed)
            finish_shell(pid, fd)
        finally:
            os.close(fd)
            try:
                exited, _ = os.waitpid(pid, os.WNOHANG)
                if not exited:
                    os.kill(pid, signal.SIGKILL)
                    os.waitpid(pid, 0)
            except ChildProcessError:
                pass
    return starts, commands


def report(label, samples):
    ordered = sorted(samples)
    values = [sum(samples) / len(samples)]
    values.extend(ordered[math.ceil(len(samples) * q) - 1] for q in (0.5, 0.95, 1))
    print(label + "\t" + "\t".join(f"{value:.3f}" for value in values))


def prompt_environment(root, home):
    # Keep project and host integration settings out of the measurement environment.
    env = {"HOME": str(home), "ZDOTDIR": os.environ["ZDOTDIR"],
           "LANG": os.environ.get("LANG", "en_US.UTF-8")}
    env.update(
        XDG_CONFIG_HOME=str(home / ".config"), XDG_DATA_HOME=str(home / ".local/share"),
        XDG_STATE_HOME=str(home / ".local/state"), XDG_CACHE_HOME=str(home / ".cache"),
        MISE_DATA_DIR=str(home / ".local/share/mise"), MISE_CACHE_DIR=str(home / ".cache/mise"),
        MISE_STATE_DIR=str(home / ".local/state/mise"),
        MISE_GLOBAL_CONFIG_FILE=str(home / ".config/mise/config.toml"), MISE_OFFLINE="1",
        STARSHIP_CONFIG=str(root / "config/shared/starship.toml"), SELFISHELL_UPDATE_NOTICE="0",
        SELFISHELL_BENCHMARK_PLATFORM_CONFIG=os.environ["SELFISHELL_BENCHMARK_PLATFORM_CONFIG"],
        PATH=os.environ["SELFISHELL_BENCHMARK_PATH"], SHELL="/bin/zsh", TERM="xterm-256color",
    )
    if "WSL_DISTRO_NAME" in os.environ:
        env["WSL_DISTRO_NAME"] = os.environ["WSL_DISTRO_NAME"]
    return env


def main():
    iterations, root = int(sys.argv[1]), Path(sys.argv[2])
    home = Path(os.environ["HOME"])
    env = prompt_environment(root, home)
    for scenario, cwd in (("empty", home), ("repository", root)):
        starts, commands = measure(cwd, env, iterations)
        report(f"prompt-first-{scenario}", starts)
        report(f"prompt-command-{scenario}", commands)


if __name__ == "__main__":
    main()
