#!/usr/bin/env bash
# build-mdlink - Compile mdlink binary or Docker image
# =====================================================
# USAGE:
#   ./build-mdlink.sh                 # Compile binary to ./mdlink
#   ./build-mdlink.sh docker          # Build Docker image
#   ./build-mdlink.sh install         # Compile + ask to install global
#
# REQUIREMENTS:
#   - go (for binary build)
#   - docker (for Docker build)
#   - mdlink.go in current dir

set -euo pipefail

readonly PROG="${0##*/}"
readonly TARGET="${1:-binary}"
readonly INSTALL_PATH="/usr/local/bin/mdlink"

die() {
  echo "$PROG: $*" >&2
  exit 1
}

# ============= BUILD BINARY =============

build_binary() {
  echo "Building mdlink binary..."

  if ! command -v go >/dev/null 2>&1; then
    die "Go not found. Run: ~/.dotfiles/ensure-go.sh"
  fi

  if [[ ! -f "mdlink.go" ]]; then
    die "mdlink.go not found in $(pwd)"
  fi

  go build -o mdlink mdlink.go
  echo "✓ Binary: ./mdlink"
}

# ============= BUILD DOCKER =============

build_docker() {
  echo "Building Docker image..."

  if ! command -v docker >/dev/null 2>&1; then
    die "Docker not found. Install Docker."
  fi

  if [[ ! -f "mdlink.go" ]]; then
    die "mdlink.go not found in $(pwd)"
  fi

  if [[ ! -f "Dockerfile" ]]; then
    die "Dockerfile not found in $(pwd)"
  fi

  docker build -t mdlink .
  echo "✓ Docker image: mdlink"
  echo ""
  echo "Usage:"
  echo "  docker run --rm -v \$(pwd):/data mdlink find /data/README.md \"url\""
}

# ============= INSTALL BINARY =============

install_binary() {
  build_binary

  echo ""
  read -p "Install mdlink to $INSTALL_PATH? [y/N]: " resp
  [[ "$resp" =~ ^[Yy]$ ]] || {
    echo "Skipped. Binary available at ./mdlink"
    return 0
  }

  echo "Installing..."
  sudo mv mdlink "$INSTALL_PATH"
  sudo chmod +x "$INSTALL_PATH"

  echo "✓ Installed: $INSTALL_PATH"
  echo ""
  echo "Test: mdlink --help"
}

# ============= MAIN =============

case "$TARGET" in
  binary)
    build_binary
    ;;
  docker)
    build_docker
    ;;
  install)
    install_binary
    ;;
  *)
    die "Usage: $PROG [binary|docker|install]"
    ;;
esac
