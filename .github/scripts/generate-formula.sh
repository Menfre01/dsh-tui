#!/usr/bin/env bash
set -euo pipefail

# generate-formula.sh — Generate Homebrew formula from dist/checksums.txt
# Usage:
#   make release        # first, build tarballs & checksums
#   # or directly:
#   .github/scripts/generate-formula.sh [version-tag] > Formula/dsh-tui.rb

DIST_DIR="${DIST_DIR:-dist}"
CHECKSUMS="${DIST_DIR}/checksums.txt"

if [ ! -f "$CHECKSUMS" ]; then
  echo "ERROR: $CHECKSUMS not found. Run 'make release' first." >&2
  exit 1
fi

VERSION="${1:-$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")}"
VERSION_NO_V="${VERSION#v}"

DARWIN_ARM64=$(grep darwin_arm64 "$CHECKSUMS" | awk '{print $1}')
DARWIN_AMD64=$(grep darwin_amd64 "$CHECKSUMS" | awk '{print $1}')
LINUX_ARM64=$(grep linux_arm64 "$CHECKSUMS" | awk '{print $1}')
LINUX_AMD64=$(grep linux_amd64 "$CHECKSUMS" | awk '{print $1}')

cat <<RUBYEOF
# typed: false
# frozen_string_literal: true

class DshTui < Formula
  desc "Terminal client for deepseek-harness (pure client, connects to a dsh host)"
  homepage "https://github.com/Menfre01/dsh-tui"
  version "${VERSION_NO_V}"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Menfre01/dsh-tui/releases/download/${VERSION}/dsh-tui_darwin_arm64.tar.gz"
      sha256 "${DARWIN_ARM64}"
    else
      url "https://github.com/Menfre01/dsh-tui/releases/download/${VERSION}/dsh-tui_darwin_amd64.tar.gz"
      sha256 "${DARWIN_AMD64}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Menfre01/dsh-tui/releases/download/${VERSION}/dsh-tui_linux_arm64.tar.gz"
      sha256 "${LINUX_ARM64}"
    else
      url "https://github.com/Menfre01/dsh-tui/releases/download/${VERSION}/dsh-tui_linux_amd64.tar.gz"
      sha256 "${LINUX_AMD64}"
    end
  end

  def install
    bin.install "dsh-tui"
  end

  test do
    assert_match "dsh-tui #{version}", shell_output("#{bin}/dsh-tui --version")
  end
end
RUBYEOF
