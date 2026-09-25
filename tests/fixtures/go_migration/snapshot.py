"""Capture exact fixture bytes, permissions and links without following symlinks."""

import json
import os
from pathlib import Path
import stat
import sys


def snapshot(root):
    entries = []

    def visit(path):
        info = path.lstat()
        entry = {
            "path": str(path.relative_to(root)),
            "type": stat.S_IFMT(info.st_mode),
            "mode": stat.S_IMODE(info.st_mode),
        }
        if stat.S_ISREG(info.st_mode):
            entry["bytes"] = path.read_bytes().hex()
        elif stat.S_ISLNK(info.st_mode):
            entry["target"] = os.readlink(path)
        entries.append(entry)
        if stat.S_ISDIR(info.st_mode):
            for child in sorted(path.iterdir()):
                visit(child)

    visit(root)
    return entries


if __name__ == "__main__":
    json.dump(snapshot(Path(sys.argv[1])), sys.stdout, indent=2, sort_keys=True)
    sys.stdout.write("\n")
