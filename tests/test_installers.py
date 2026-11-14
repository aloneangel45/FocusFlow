"""Verification tests for universal one-line installers (`install.sh` and `install.ps1`)."""

from __future__ import annotations

import functools
import hashlib
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import os
from pathlib import Path
import shutil
import subprocess
import sys
import threading
import zipfile

import pytest


class _QuietHandler(SimpleHTTPRequestHandler):
    def log_message(self, format: str, *args: object) -> None:  # noqa: A002
        return


def test_install_sh_lf_syntax_and_release_asset_alignment() -> None:
    repo_root = Path(__file__).resolve().parent.parent
    install_sh = repo_root / "install.sh"
    raw_bytes = install_sh.read_bytes()

    # POSIX shell script must use LF line endings so `curl ... | sh` never breaks on CR
    assert b"\r\n" not in raw_bytes, "install.sh must use strict LF line endings"
    text = raw_bytes.decode("utf-8")

    assert text.startswith("#!/usr/bin/env sh\n")
    assert 'BIN_INNER="focusflow-${OS_NAME}-${ARCH_NAME}"' in text
    assert 'ARCHIVE_NAME="${BIN_INNER}.tar.gz"' in text
    assert "checksums.txt" in text
    assert "SHA-256 checksum verification FAILED" in text
    # Verify checksum check precedes tar extraction
    assert text.index("SHA-256 checksum verification FAILED") < text.index("tar -xzf")


@pytest.mark.skipif(sys.platform != "win32", reason="PowerShell installer end-to-end test runs on Windows")
def test_install_ps1_end_to_end_sha256_verification(tmp_path: Path) -> None:
    pwsh = shutil.which("powershell") or shutil.which("pwsh")
    if pwsh is None:
        pytest.skip("PowerShell not available")

    repo_root = Path(__file__).resolve().parent.parent
    install_ps1 = repo_root / "install.ps1"

    serve_dir = tmp_path / "releases"
    serve_dir.mkdir(parents=True)
    install_dir = tmp_path / "installed_bin"

    # Build a tiny valid Windows batch/exe stub or compile real focusflow.exe if Go is present
    go_bin = shutil.which("go") or (
        str(Path(r"C:\Program Files\Go\bin\go.exe")) if Path(r"C:\Program Files\Go\bin\go.exe").exists() else None
    )
    arch = "arm64" if os.environ.get("PROCESSOR_ARCHITECTURE", "").upper() == "ARM64" else "amd64"
    inner_name = f"focusflow-windows-{arch}.exe"
    archive_name = f"focusflow-windows-{arch}.zip"

    built_exe = tmp_path / inner_name
    if go_bin is not None:
        subprocess.run(
            [go_bin, "build", "-o", str(built_exe), "./cmd/focusflow"],
            cwd=str(repo_root),
            check=True,
        )
    else:
        pytest.skip("Go compiler required to build real test binary for install.ps1")

    archive_path = serve_dir / archive_name
    with zipfile.ZipFile(archive_path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        zf.write(built_exe, arcname=inner_name)

    good_sha256 = hashlib.sha256(archive_path.read_bytes()).hexdigest()
    checksums_file = serve_dir / "checksums.txt"
    checksums_file.write_text(f"{good_sha256}  {archive_name}\n", encoding="utf-8")

    handler = functools.partial(_QuietHandler, directory=str(serve_dir))
    server = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    port = server.server_address[1]
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()

    try:
        env = os.environ.copy()
        env["FOCUSFLOW_BASE_URL"] = f"http://127.0.0.1:{port}"
        env["FOCUSFLOW_INSTALL_DIR"] = str(install_dir)
        env["FOCUSFLOW_NO_PATH_UPDATE"] = "1"

        # 1. Valid SHA-256 checksum -> installs focusflow.exe and prints version
        ok_run = subprocess.run(
            [pwsh, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(install_ps1)],
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
        assert ok_run.returncode == 0, f"install.ps1 failed:\nSTDOUT:\n{ok_run.stdout}\nSTDERR:\n{ok_run.stderr}"
        installed_exe = install_dir / "focusflow.exe"
        assert installed_exe.exists()
        assert "Verified SHA-256" in ok_run.stdout
        assert "FocusFlow v2.2.0" in ok_run.stdout

        # 2. Tampered SHA-256 checksum -> MUST abort with non-zero exit code without overwriting/installing!
        tampered_dir = tmp_path / "tampered_install"
        env["FOCUSFLOW_INSTALL_DIR"] = str(tampered_dir)
        checksums_file.write_text(f"{'0' * 64}  {archive_name}\n", encoding="utf-8")

        bad_run = subprocess.run(
            [pwsh, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(install_ps1)],
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
        assert bad_run.returncode != 0
        assert not (tampered_dir / "focusflow.exe").exists()
    finally:
        server.shutdown()
        server.server_close()
