#!/usr/bin/env python3
"""Write snoop's PyPI wheels from the binaries scripts/build_binaries.sh built.

Each wheel holds one platform's binary as a script, so pip, pipx and uv put
`snoop` on PATH; there is no Python code in it. The binaries are static, so
one Linux wheel serves both glibc and musl.

    scripts/wheels.py <version> [dist]

Writes dist/wheels/snoop_cli-<version>-py3-none-<platform>.whl. Standard
library only, so it runs on any runner without installing anything.
"""

import base64
import hashlib
import os
import re
import subprocess
import sys
import zipfile

NAME = "snoop-cli"
DIST = "snoop_cli"
SUMMARY = (
    "A terminal viewer for Claude Code sessions: follow one live, or see what "
    "any past one cost, changed and spent its time on."
)

# Go's target, and the wheel's platform tag.
TARGETS = [
    ("linux_amd64", "manylinux_2_17_x86_64.musllinux_1_1_x86_64"),
    ("linux_arm64", "manylinux_2_17_aarch64.musllinux_1_1_aarch64"),
    ("darwin_amd64", "macosx_10_12_x86_64"),
    ("darwin_arm64", "macosx_11_0_arm64"),
    ("windows_amd64", "win_amd64"),
    ("windows_arm64", "win_arm64"),
]

# A fixed time, so the same binaries always make the same wheel.
STAMP = (2020, 1, 1, 0, 0, 0)


def pep440(version):
    """Turns a release version into PEP 440: 1.2.3-dev becomes 1.2.3.dev0."""
    version = version.removeprefix("v")
    m = re.fullmatch(r"(\d+\.\d+\.\d+)(?:-([a-z]+)\.?(\d*))?", version)
    if not m:
        sys.exit(f"wheels.py: can't express version {version!r} for PyPI")
    base, pre, n = m.groups()
    if not pre:
        return base
    pre = {"alpha": "a", "beta": "b", "rc": "rc"}.get(pre, pre)
    if pre in ("a", "b", "rc"):
        return f"{base}{pre}{n or 0}"
    return f"{base}.dev{n or 0}"


def record_hash(data):
    digest = base64.urlsafe_b64encode(hashlib.sha256(data).digest())
    return "sha256=" + digest.rstrip(b"=").decode()


def wheel(out, root, dist, version, target, tag):
    exe = "snoop.exe" if target.startswith("windows") else "snoop"
    binary = os.path.join(dist, "binaries", f"snoop_{target}", exe)
    if not os.path.isfile(binary):
        sys.exit(f"wheels.py: missing {binary}")
    with open(binary, "rb") as f:
        body = f.read()
    # The README with absolute links, as PyPI shows it.
    readme = subprocess.run([os.path.join(root, "scripts", "package_readme.sh")],
                            check=True, capture_output=True, text=True).stdout
    with open(os.path.join(root, "LICENSE"), "rb") as f:
        license_text = f.read()

    info = f"{DIST}-{version}.dist-info"
    metadata = "\n".join([
        "Metadata-Version: 2.1",
        f"Name: {NAME}",
        f"Version: {version}",
        f"Summary: {SUMMARY}",
        "License: MIT",
        "Project-URL: Homepage, https://jzills.github.io/snoop/",
        "Project-URL: Source, https://github.com/jzills/snoop",
        "Classifier: Environment :: Console",
        "Classifier: License :: OSI Approved :: MIT License",
        "Requires-Python: >=3.8",
        "Description-Content-Type: text/markdown",
        "",
        readme,
    ])
    tags = "".join(f"Tag: py3-none-{t}\n" for t in tag.split("."))
    wheel_info = f"Wheel-Version: 1.0\nGenerator: snoop scripts/wheels.py\nRoot-Is-Purelib: false\n{tags}"

    files = [
        (f"{DIST}-{version}.data/scripts/{exe}", body, 0o755),
        (f"{info}/METADATA", metadata.encode(), 0o644),
        (f"{info}/WHEEL", wheel_info.encode(), 0o644),
        (f"{info}/LICENSE", license_text, 0o644),
    ]
    record = "".join(f"{name},{record_hash(data)},{len(data)}\n" for name, data, _ in files)
    record += f"{info}/RECORD,,\n"
    files.append((f"{info}/RECORD", record.encode(), 0o644))

    path = os.path.join(out, f"{DIST}-{version}-py3-none-{tag}.whl")
    with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as z:
        for name, data, mode in files:
            zi = zipfile.ZipInfo(name, STAMP)
            zi.external_attr = (0o100000 | mode) << 16
            zi.compress_type = zipfile.ZIP_DEFLATED
            z.writestr(zi, data)
    return path


def main():
    if len(sys.argv) not in (2, 3):
        sys.exit("usage: wheels.py <version> [dist]")
    version = pep440(sys.argv[1])
    dist = sys.argv[2] if len(sys.argv) == 3 else "dist"
    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    out = os.path.join(dist, "wheels")
    os.makedirs(out, exist_ok=True)
    for old in os.listdir(out):
        if old.endswith(".whl"):
            os.remove(os.path.join(out, old))
    for target, tag in TARGETS:
        print(wheel(out, root, dist, version, target, tag))


if __name__ == "__main__":
    main()
