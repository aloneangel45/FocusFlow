"""Hermetic test fixtures ensuring no test ever touches real %APPDATA% or system hosts."""

from __future__ import annotations

from pathlib import Path
import pytest


@pytest.fixture(autouse=True)
def hermetic_focusflow_env(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> tuple[Path, Path]:
    state_dir = tmp_path / "focusflow_state"
    state_dir.mkdir(parents=True, exist_ok=True)
    hosts_file = tmp_path / "hosts"
    hosts_file.write_text("127.0.0.1 localhost\n::1 localhost\n", encoding="utf-8")

    monkeypatch.setenv("FOCUSFLOW_HOME", str(state_dir))
    monkeypatch.setenv("FOCUSFLOW_HOSTS_PATH", str(hosts_file))
    return state_dir, hosts_file
