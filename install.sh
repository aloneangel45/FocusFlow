#!/usr/bin/env sh
# FocusFlow Universal POSIX Installer (Linux & macOS amd64/arm64)
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/aloneangel45/FocusFlow/main/install.sh | sh
# Optional environment variables:
#   FOCUSFLOW_VERSION     Release tag to install (e.g. "v2.2.1" or "latest", default: "latest")
#   FOCUSFLOW_INSTALL_DIR Target directory for the focusflow binary
#   FOCUSFLOW_REPO        GitHub owner/repo (default: "aloneangel45/FocusFlow")
#   FOCUSFLOW_BASE_URL    Override release download base URL (for mirrors/testing)

set -eu

REPO="${FOCUSFLOW_REPO:-aloneangel45/FocusFlow}"
VERSION="${FOCUSFLOW_VERSION:-latest}"

log_info() {
    printf '[INFO] %s\n' "$*"
}

log_ok() {
    printf '[OK] %s\n' "$*"
}

log_err() {
    printf '[ERR] %s\n' "$*" >&2
}

detect_os() {
    raw_os="$(uname -s 2>/dev/null || echo unknown)"
    case "$raw_os" in
        Linux|linux)
            printf 'linux'
            ;;
        Darwin|darwin)
            printf 'darwin'
            ;;
        *)
            log_err "Unsupported operating system: $raw_os (expected Linux or macOS)."
            exit 1
            ;;
    esac
}

detect_arch() {
    raw_arch="$(uname -m 2>/dev/null || echo unknown)"
    case "$raw_arch" in
        x86_64|amd64)
            printf 'amd64'
            ;;
        aarch64|arm64)
            printf 'arm64'
            ;;
        *)
            log_err "Unsupported CPU architecture: $raw_arch (expected amd64/x86_64 or arm64/aarch64)."
            exit 1
            ;;
    esac
}

download_file() {
    src_url="$1"
    dest_path="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$src_url" -o "$dest_path"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$dest_path" "$src_url"
    else
        log_err "Neither 'curl' nor 'wget' was found. Install curl or wget and retry."
        exit 1
    fi
}

compute_sha256() {
    target_file="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$target_file" | awk '{print tolower($1)}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$target_file" | awk '{print tolower($1)}'
    elif command -v openssl >/dev/null 2>&1; then
        openssl dgst -sha256 "$target_file" | awk '{print tolower($NF)}'
    else
        log_err "No SHA-256 utility found (requires sha256sum, shasum, or openssl)."
        exit 1
    fi
}

resolve_base_url() {
    if [ -n "${FOCUSFLOW_BASE_URL:-}" ]; then
        printf '%s' "${FOCUSFLOW_BASE_URL%/}"
        return
    fi
    trimmed_ver="$(printf '%s' "$VERSION" | tr -d '[:space:]')"
    lower_ver="$(printf '%s' "$trimmed_ver" | tr '[:upper:]' '[:lower:]')"
    if [ -z "$trimmed_ver" ] || [ "$lower_ver" = "latest" ]; then
        printf 'https://github.com/%s/releases/latest/download' "$REPO"
    else
        clean_ver="${trimmed_ver#[vV]}"
        printf 'https://github.com/%s/releases/download/v%s' "$REPO" "$clean_ver"
    fi
}

resolve_install_dir() {
    if [ -n "${FOCUSFLOW_INSTALL_DIR:-}" ]; then
        printf '%s' "$FOCUSFLOW_INSTALL_DIR"
        return
    fi
    if [ "$(id -u)" -eq 0 ] || [ -w "/usr/local/bin" ]; then
        printf '/usr/local/bin'
    else
        printf '%s/.local/bin' "$HOME"
    fi
}

main() {
    OS_NAME="$(detect_os)"
    ARCH_NAME="$(detect_arch)"
    BIN_INNER="focusflow-${OS_NAME}-${ARCH_NAME}"
    ARCHIVE_NAME="${BIN_INNER}.tar.gz"
    BASE_URL="$(resolve_base_url)"
    INSTALL_DIR="$(resolve_install_dir)"

    TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t focusflow-install)"
    trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

    ARCHIVE_PATH="${TMP_DIR}/${ARCHIVE_NAME}"
    CHECKSUMS_PATH="${TMP_DIR}/checksums.txt"

    log_info "Detected platform: ${OS_NAME}/${ARCH_NAME}"
    log_info "Downloading ${ARCHIVE_NAME} from ${BASE_URL}..."
    download_file "${BASE_URL}/${ARCHIVE_NAME}" "$ARCHIVE_PATH"

    log_info "Downloading checksums.txt for mandatory SHA-256 verification..."
    download_file "${BASE_URL}/checksums.txt" "$CHECKSUMS_PATH"

    EXPECTED_HASH="$(awk -v asset="$ARCHIVE_NAME" '
        {
            sub(/^\357\273\277/, "", $0)
            sub(/\r$/, "", $0)
        }
        NF >= 2 {
            fname = $2
            sub(/^\*/, "", fname)
            if (fname == asset) {
                hash = tolower($1)
                if (hash ~ /^[0-9a-f]+$/ && length(hash) == 64) {
                    print hash
                    exit
                }
            }
        }
    ' "$CHECKSUMS_PATH")"

    if [ -z "$EXPECTED_HASH" ] || [ "${#EXPECTED_HASH}" -ne 64 ]; then
        log_err "Could not find valid 64-char SHA-256 entry for ${ARCHIVE_NAME} in checksums.txt."
        exit 1
    fi

    ACTUAL_HASH="$(compute_sha256 "$ARCHIVE_PATH")"
    if [ "$ACTUAL_HASH" != "$EXPECTED_HASH" ]; then
        log_err "SHA-256 checksum verification FAILED for ${ARCHIVE_NAME}!"
        log_err "  Expected: ${EXPECTED_HASH}"
        log_err "  Actual:   ${ACTUAL_HASH}"
        exit 1
    fi
    log_ok "Verified SHA-256: ${ACTUAL_HASH}"

    tar -xzf "$ARCHIVE_PATH" -C "$TMP_DIR"
    EXTRACTED_BIN="${TMP_DIR}/${BIN_INNER}"
    if [ ! -f "$EXTRACTED_BIN" ]; then
        log_err "Expected binary '${BIN_INNER}' not found inside ${ARCHIVE_NAME}."
        exit 1
    fi

    mkdir -p "$INSTALL_DIR"
    TARGET_BIN="${INSTALL_DIR}/focusflow"
    cp "$EXTRACTED_BIN" "$TARGET_BIN"
    chmod 0755 "$TARGET_BIN"

    log_ok "Installed FocusFlow binary to ${TARGET_BIN}"
    "$TARGET_BIN" version || true

    case ":${PATH:-}:" in
        *":${INSTALL_DIR}:"*)
            ;;
        *)
            log_info "Add ${INSTALL_DIR} to your PATH if not already configured:"
            log_info "  export PATH=\"${INSTALL_DIR}:\$PATH\""
            ;;
    esac
}

main "$@"
