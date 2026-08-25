#!/usr/bin/env bash
# 由 GitHub Release 的 checksums.txt 產生 Homebrew Formula。
#
# 用法：gen_formula.sh <version-without-v> <checksums.txt> <output.rb>
set -euo pipefail

VER="${1:?usage: gen_formula.sh <version> <checksums.txt> <output.rb>}"
IN="${2:?missing checksums.txt}"
OUT="${3:?missing output path}"
OWNER="${OWNER:-gentoobreaking}"

sha_of() { grep "_$1" "$IN" | awk '{print $1}'; }

SHA_DARWIN_ARM64=$(sha_of darwin_arm64)
SHA_DARWIN_AMD64=$(sha_of darwin_amd64)
SHA_LINUX_ARM64=$(sha_of linux_arm64)
SHA_LINUX_AMD64=$(sha_of linux_amd64)
BASE_URL="https://github.com/${OWNER}/tw-quant-mcp/releases/download/v${VER}"

mkdir -p "$(dirname "$OUT")"
cat >"$OUT" <<FORMULA
# 自動產生：由 tw-quant-mcp release workflow 於 ${VER} 發佈時更新，請勿手改。
class TwQuantMcp < Formula
  desc "Taiwan quant market data MCP Server (official sources)"
  homepage "https://github.com/${OWNER}/tw-quant-mcp"
  version "${VER}"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "${BASE_URL}/tw-quant-mcp_v#{version}_darwin_arm64.tar.gz"
      sha256 "${SHA_DARWIN_ARM64}"
    else
      url "${BASE_URL}/tw-quant-mcp_v#{version}_darwin_amd64.tar.gz"
      sha256 "${SHA_DARWIN_AMD64}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "${BASE_URL}/tw-quant-mcp_v#{version}_linux_arm64.tar.gz"
      sha256 "${SHA_LINUX_ARM64}"
    else
      url "${BASE_URL}/tw-quant-mcp_v#{version}_linux_amd64.tar.gz"
      sha256 "${SHA_LINUX_AMD64}"
    end
  end

  def install
    os = OS.mac? ? "darwin" : "linux"
    arch = Hardware::CPU.arm? ? "arm64" : "amd64"
    bin.install "tw-quant-mcp_v#{version}_#{os}_#{arch}" => "tw-quant-mcp"
  end

  def caveats
    <<~CAVEATS
      MCP stdio server. Point your MCP client to:
        #{HOMEBREW_PREFIX}/bin/tw-quant-mcp

      Example (Claude Desktop):
        { "mcpServers": { "tw-quant-mcp": { "command": "#{HOMEBREW_PREFIX}/bin/tw-quant-mcp" } } }
    CAVEATS
  end

  test do
    require "open3"
    input = '{"jsonrpc":"2.0","id":1,"method":"initialize",' \
            '"params":{"protocolVersion":"2025-03-26","capabilities":{},' \
            '"clientInfo":{"name":"brew-test","version":"1"}}}'
    out, err, status = Open3.capture3(bin/"tw-quant-mcp", stdin_data: input + "\n")
    assert status.success?, "tw-quant-mcp exited abnormally: #{err}"
    assert_match %r{serverInfo}m, out, "initialize 回應應含 serverInfo"
  end
end
FORMULA

echo "formula written: $OUT"
