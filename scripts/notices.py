#!/usr/bin/env python3
"""Generate notices from pinned module licenses; never substitute the root license."""
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def main():
    subprocess.run(["go", "mod", "download"], cwd=ROOT, check=True)
    raw = subprocess.check_output(["go", "list", "-m", "-json", "all"], cwd=ROOT, text=True)
    decoder = json.JSONDecoder()
    parts = ["# Third-party notices\n\nLicenses copied from the pinned Go modules.\n"]
    while raw.strip():
        module, end = decoder.raw_decode(raw.lstrip())
        raw = raw.lstrip()[end:]
        if module.get("Main"):
            continue
        if not module.get("Dir"):
            module = json.loads(subprocess.check_output(
                ["go", "mod", "download", "-json", module["Path"] + "@" + module["Version"]],
                cwd=ROOT, text=True))
        directory = Path(module["Dir"])
        files = [directory / name for name in ("LICENSE", "LICENSE.md", "LICENSE.txt", "COPYING")]
        license_path = next((path for path in files if path.is_file()), None)
        if license_path is None:
            raise RuntimeError("Missing license: " + module["Path"])
        parts.append("\n## " + module["Path"] + " " + module["Version"] + "\n\n" + license_path.read_text())
    goroot = Path(subprocess.check_output(["go", "env", "GOROOT"], cwd=ROOT, text=True).strip())
    parts.append("\n## Go standard library\n\n" + (goroot / "LICENSE").read_text())
    (ROOT / "THIRD_PARTY_NOTICES.md").write_text("\n".join(parts))


if __name__ == "__main__":
    main()
