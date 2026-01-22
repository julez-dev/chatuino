#!/usr/bin/env sh
# Chatuino install script
# Usage: curl -sSfL https://chatuino.net/install | sh
#    or: curl -sSfL https://chatuino.net/install | sh -s -- -b /usr/local/bin

set -eu

BOLD="$(tput bold 2>/dev/null || printf '')"
RED="$(tput setaf 1 2>/dev/null || printf '')"
GREEN="$(tput setaf 2 2>/dev/null || printf '')"
YELLOW="$(tput setaf 3 2>/dev/null || printf '')"
BLUE="$(tput setaf 4 2>/dev/null || printf '')"
RESET="$(tput sgr0 2>/dev/null || printf '')"

GITHUB_REPO="julez-dev/chatuino"
BINARY_NAME="chatuino"

info() {
    printf '%s\n' "${BOLD}>${RESET} $*"
}

warn() {
    printf '%s\n' "${YELLOW}! $*${RESET}"
}

error() {
    printf '%s\n' "${RED}x $*${RESET}" >&2
}

success() {
    printf '%s\n' "${GREEN}✓${RESET} $*"
}

has() {
    command -v "$1" >/dev/null 2>&1
}

usage() {
    cat <<EOF
Install chatuino - Terminal UI Twitch IRC client

Usage: install.sh [options]

Options:
    -b, --bin-dir DIR    Install directory (default: ~/.local/bin)
    -v, --version VER    Install specific version (default: latest)
    -f, -y, --force      Skip confirmation prompt
    -h, --help           Show this help

Examples:
    curl -sSfL https://chatuino.net/install | sh
    curl -sSfL https://chatuino.net/install | sh -s -- -b /usr/local/bin
    curl -sSfL https://chatuino.net/install | sh -s -- -v v0.6.2
EOF
}

detect_os() {
    os="$(uname -s)"
    case "$os" in
        Linux*)  printf "Linux" ;;
        Darwin*) printf "Darwin" ;;
        *)
            error "Unsupported OS: $os"
            exit 1
            ;;
    esac
}

detect_arch() {
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)   printf "x86_64" ;;
        arm64|aarch64)  printf "arm64" ;;
        *)
            error "Unsupported architecture: $arch"
            exit 1
            ;;
    esac
}

# Check if chatuino is managed by a package manager
check_package_manager() {
    existing_bin="$1"

    # Check pacman (Arch Linux / AUR)
    if has pacman; then
        if pacman -Qo "$existing_bin" >/dev/null 2>&1; then
            pkg_name=$(pacman -Qoq "$existing_bin" 2>/dev/null)
            error "chatuino is already managed by pacman (package: $pkg_name)"
            error "Using this script may cause conflicts with your package manager"
            exit 1
        fi
    fi

    # Check Homebrew
    if has brew; then
        if brew list --formula chatuino >/dev/null 2>&1; then
            error "chatuino is already managed by Homebrew"
            error "Using this script may cause conflicts with your package manager"
            exit 1
        fi
    fi
}

get_latest_version() {
    url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"

    if has curl; then
        curl -sSfL "$url" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    elif has wget; then
        wget -qO- "$url" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    else
        error "Neither curl nor wget found"
        exit 1
    fi
}

download() {
    url="$1"
    output="$2"

    if has curl; then
        curl -sSfL -o "$output" "$url"
    elif has wget; then
        wget -qO "$output" "$url"
    else
        error "Neither curl nor wget found"
        exit 1
    fi
}

download_with_progress() {
    url="$1"
    output="$2"

    if has curl; then
        # -# shows progress bar, -f fails on HTTP errors, -L follows redirects
        curl -#fL -o "$output" "$url"
    elif has wget; then
        # wget shows progress by default
        wget -O "$output" "$url"
    else
        error "Neither curl nor wget found"
        exit 1
    fi
}

verify_checksum() {
    archive="$1"
    checksums_file="$2"
    archive_name="$3"

    if has sha256sum; then
        expected=$(grep -F "$archive_name" "$checksums_file" | awk '{print $1}')
        actual=$(sha256sum "$archive" | awk '{print $1}')
    elif has shasum; then
        expected=$(grep -F "$archive_name" "$checksums_file" | awk '{print $1}')
        actual=$(shasum -a 256 "$archive" | awk '{print $1}')
    else
        warn "Neither sha256sum nor shasum found, skipping checksum verification"
        return 0
    fi

    if [ -z "$expected" ]; then
        error "Checksum not found for $archive_name"
        exit 1
    fi

    if [ "$expected" != "$actual" ]; then
        error "Checksum verification failed"
        error "Expected: $expected"
        error "Actual:   $actual"
        exit 1
    fi

    success "Checksum verified"
}

check_path() {
    bin_dir="$1"

    case ":$PATH:" in
        *":$bin_dir:"*) return 0 ;;
    esac

    warn "$bin_dir is not in your PATH"
    printf '\n'

    shell_name=$(basename "${SHELL:-sh}")
    case "$shell_name" in
        bash)  rc_file="~/.bashrc" ;;
        zsh)   rc_file="~/.zshrc" ;;
        fish)  rc_file="~/.config/fish/config.fish" ;;
        *)     rc_file="your shell's rc file" ;;
    esac

    if [ "$shell_name" = "fish" ]; then
        path_cmd="fish_add_path $bin_dir"
    else
        path_cmd="export PATH=\"$bin_dir:\$PATH\""
    fi

    info "Add to $rc_file:"
    printf '\n    %s\n\n' "${BLUE}${path_cmd}${RESET}"

    if [ -z "${FORCE:-}" ]; then
        printf '%s ' "${BOLD}Add to $rc_file now? [y/N]${RESET}"
        read -r reply </dev/tty || reply="n"

        if [ "$reply" = "y" ] || [ "$reply" = "Y" ]; then
            # Expand ~ for the actual file path
            case "$shell_name" in
                bash)  actual_rc="$HOME/.bashrc" ;;
                zsh)   actual_rc="$HOME/.zshrc" ;;
                fish)  actual_rc="$HOME/.config/fish/config.fish" ;;
                *)     actual_rc="" ;;
            esac

            if [ -n "$actual_rc" ]; then
                # Ensure directory exists for fish
                if [ "$shell_name" = "fish" ]; then
                    mkdir -p "$(dirname "$actual_rc")"
                fi

                printf '\n# Added by chatuino installer\n%s\n' "$path_cmd" >> "$actual_rc"
                success "Added to $rc_file"
                info "Run 'source $rc_file' or start a new shell"
            else
                warn "Unknown shell, please add manually"
            fi
        fi
    fi
}

confirm() {
    if [ -z "${FORCE:-}" ]; then
        printf '%s ' "${BOLD}$* [y/N]${RESET}"
        read -r reply </dev/tty || reply="n"
        if [ "$reply" != "y" ] && [ "$reply" != "Y" ]; then
            error "Aborted"
            exit 1
        fi
    fi
}

main() {
    BIN_DIR="${HOME}/.local/bin"
    VERSION=""
    FORCE=""

    while [ $# -gt 0 ]; do
        case "$1" in
            -b|--bin-dir)
                [ -z "${2:-}" ] && { error "-b requires an argument"; exit 1; }
                BIN_DIR="$2"
                shift 2
                ;;
            -v|--version)
                [ -z "${2:-}" ] && { error "-v requires an argument"; exit 1; }
                VERSION="$2"
                shift 2
                ;;
            -f|-y|--force|--yes)
                FORCE=1
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done

    printf '\n'
    info "${BOLD}Chatuino Installer${RESET}"
    printf '\n'

    # Detect platform
    OS=$(detect_os)
    ARCH=$(detect_arch)

    info "Detected: ${GREEN}${OS}${RESET} / ${GREEN}${ARCH}${RESET}"

    # Check for existing installation via package manager
    existing_bin=$(command -v "$BINARY_NAME" 2>/dev/null || true)
    if [ -n "$existing_bin" ]; then
        check_package_manager "$existing_bin"
        info "Existing installation found: $existing_bin (will be replaced)"
    fi

    # Get version
    if [ -z "$VERSION" ]; then
        info "Fetching latest version..."
        VERSION=$(get_latest_version)
        if [ -z "$VERSION" ]; then
            error "Failed to fetch latest version from GitHub"
            exit 1
        fi
    fi

    # Ensure version starts with 'v'
    case "$VERSION" in
        v*) ;;
        *)  VERSION="v$VERSION" ;;
    esac

    info "Version: ${GREEN}${VERSION}${RESET}"
    info "Install directory: ${GREEN}${BIN_DIR}${RESET}"

    # Build artifact name
    ARCHIVE_NAME="chatuino_${OS}_${ARCH}.tar.gz"
    VERSION_NUM="${VERSION#v}"
    CHECKSUMS_NAME="chatuino_${VERSION_NUM}_checksums.txt"

    DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/${ARCHIVE_NAME}"
    CHECKSUMS_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/${CHECKSUMS_NAME}"

    printf '\n'
    confirm "Install chatuino ${VERSION} to ${BIN_DIR}?"

    # Create bin directory
    mkdir -p "$BIN_DIR"

    # Create temp directory
    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT

    # Download checksums
    info "Downloading checksums..."
    download "$CHECKSUMS_URL" "$TMP_DIR/checksums.txt"

    # Download archive
    info "Downloading ${ARCHIVE_NAME}..."
    download_with_progress "$DOWNLOAD_URL" "$TMP_DIR/$ARCHIVE_NAME"

    # Verify checksum
    verify_checksum "$TMP_DIR/$ARCHIVE_NAME" "$TMP_DIR/checksums.txt" "$ARCHIVE_NAME"

    # Extract
    info "Extracting..."
    tar -xzf "$TMP_DIR/$ARCHIVE_NAME" -C "$TMP_DIR"

    # Install
    if [ -w "$BIN_DIR" ]; then
        mv "$TMP_DIR/$BINARY_NAME" "$BIN_DIR/$BINARY_NAME"
        chmod +x "$BIN_DIR/$BINARY_NAME"
    else
        warn "Elevated permissions required for $BIN_DIR"
        if ! has sudo; then
            error "sudo not found; install manually or choose a writable directory with -b"
            exit 1
        fi
        sudo mv "$TMP_DIR/$BINARY_NAME" "$BIN_DIR/$BINARY_NAME"
        sudo chmod +x "$BIN_DIR/$BINARY_NAME"
    fi

    printf '\n'
    success "Installed chatuino ${VERSION} to ${BIN_DIR}/${BINARY_NAME}"

    # Check PATH
    check_path "$BIN_DIR"

    printf '\n'
    info "Run ${BLUE}chatuino${RESET} to get started"
    printf '\n'
}

main "$@"
