# Homebrew Formula for bootc-man
# This file is a template for the Homebrew Tap repository.
# After creating the tnk4on/homebrew-bootc-man repository,
# place this file at Formula/bootc-man.rb in that repository.
#
# Reference: Podman's formula at
# https://github.com/Homebrew/homebrew-core/blob/master/Formula/p/podman.rb

class BootcMan < Formula
  desc "CLI tool for bootable container image testing and verification"
  homepage "https://github.com/tnk4on/bootc-man"
  url "https://github.com/tnk4on/bootc-man/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "PLACEHOLDER_SHA256"
  license "Apache-2.0"

  # Bottle block is automatically updated by CI (bottle.yml in homebrew-bootc-man)
  # bottle do
  #   root_url "https://github.com/tnk4on/bootc-man/releases/download/v0.1.0"
  #   sha256 cellar: :any_skip_relocation, arm64_sequoia: "PLACEHOLDER"
  # end

  depends_on "go" => :build
  depends_on :macos
  depends_on "podman"

  # gvproxy (same pattern as Podman formula)
  resource "gvproxy" do
    on_macos do
      url "https://github.com/containers/gvisor-tap-vsock/archive/refs/tags/v0.8.7.tar.gz"
      sha256 "PLACEHOLDER_GVPROXY_SHA256"
    end
  end

  # vfkit (same pattern as Podman formula)
  resource "vfkit" do
    on_macos do
      url "https://github.com/crc-org/vfkit/archive/refs/tags/v0.6.1.tar.gz"
      sha256 "PLACEHOLDER_VFKIT_SHA256"
    end
  end

  def install
    system "make", "build", "VERSION=#{version}"
    bin.install "bin/bootc-man"

    # Build and install gvproxy
    resource("gvproxy").stage do
      system "make", "gvproxy"
      (libexec/"bootc-man").install "bin/gvproxy"
    end

    # Build and install vfkit
    resource("vfkit").stage do
      ENV["CGO_ENABLED"] = "1"
      arch = Hardware::CPU.intel? ? "amd64" : Hardware::CPU.arch.to_s
      system "make", "out/vfkit-#{arch}"
      (libexec/"bootc-man").install "out/vfkit-#{arch}" => "vfkit"
    end

    # Shell completions
    generate_completions_from_executable(bin/"bootc-man", "completion")
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/bootc-man version")
  end
end
