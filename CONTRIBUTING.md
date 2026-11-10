# Contributing to FocusFlow

FocusFlow is an open-source deep work systems utility built on a unified **Go CLI** + **zero-dependency Python Desktop HUD** architecture. Bug reports, security hardening improvements, and pull requests are welcome.

## Development Prerequisites

- **Go 1.23+** (`CGO_ENABLED=0` standard-library build)
- **Python 3.10+** (standard library only at runtime; `pytest` for running tests)
- **Git**

## Setup

```bash
git clone https://github.com/aloneangel45/FocusFlow.git
cd FocusFlow
python -m pip install -e ".[dev]"
```

## Running the Hermetic Test Suites

All tests isolate filesystem state via `FOCUSFLOW_HOME` and `FOCUSFLOW_HOSTS_PATH` temporary directories and must never modify real `%APPDATA%` or the host OS `hosts` file:

```bash
# Go static analysis and unit tests
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test -v ./...

# Python unit and cross-runtime interop tests
python -m pytest -v
```

## Contribution Guidelines

1. Fork the repository and create a topic branch (`git checkout -b feat/your-feature`).
2. Keep Go (`pkg/*`, `cmd/focusflow`) and Python (`focusflow_core/*`, `FocusFlowGUI.py`) schemas and behavior aligned whenever modifying state files (`config.json`, `sessions.json`, `hosts.focusflow.journal.json`) or `hosts` block markers.
3. Follow the zero-emoji minimal systems aesthetic across code, CLI tags, GUI labels, and documentation.
4. Ensure both `go test -v ./...` and `python -m pytest -v` pass cleanly.
5. Format commit messages according to Conventional Commits (`feat:`, `fix:`, `docs:`, `test:`, `ci:`, `chore:`) and open a Pull Request.
