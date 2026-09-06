#!/usr/bin/env python3
"""Package only public documentation and the explicitly built portable binaries."""
import gzip
import hashlib
from pathlib import Path
import tarfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
DEST = ROOT / "dist" / "releases"
DOCS = ["LICENSE", "THIRD_PARTY_NOTICES.md", "README.md", "SECURITY.md",
        "docs/deployment.md", "docs/compatibility.md"]


def main():
    DEST.mkdir(parents=True, exist_ok=True)
    archives = []
    for system in ("linux", "darwin", "windows"):
        for architecture in ("amd64", "arm64"):
            target = f"{system}-{architecture}"
            suffix = ".exe" if system == "windows" else ""
            files = [(ROOT / "dist" / target / (name + suffix), name + suffix)
                     for name in ("mcp-host-bridge", "mcp-bridge-demo")]
            files += [(ROOT / name, name) for name in DOCS]
            assert all(p.is_file() for p, _ in files), "run scripts/build.sh first"
            archive = DEST / (f"mcp-host-bridge-v0.1.0-{target}" + (".zip" if system == "windows" else ".tar.gz"))
            if system == "windows":
                with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as output:
                    for path, name in files:
                        info = zipfile.ZipInfo(name, date_time=(2026, 1, 1, 0, 0, 0))
                        info.compress_type = zipfile.ZIP_DEFLATED
                        output.writestr(info, path.read_bytes())
            else:
                with archive.open("wb") as raw, gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=0) as compressed:
                    with tarfile.open(fileobj=compressed, mode="w") as output:
                        for path, name in files:
                            info = output.gettarinfo(str(path), arcname=name)
                            info.uid = info.gid = info.mtime = 0
                            info.uname = info.gname = ""
                            with path.open("rb") as source:
                                output.addfile(info, source)
            archives.append(archive)
    sums = [f"{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}" for p in archives]
    (DEST / "SHA256SUMS").write_text("\n".join(sums) + "\n")
    print(f"Packaged {len(archives)} archives with licenses and SHA256SUMS")


if __name__ == "__main__":
    main()
