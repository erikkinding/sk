class Sk < Formula
  desc "Switch Kontext - quickly move between Kubernetes contexts and namespaces"
  homepage "https://github.com/erikkinding/sk"
  url "https://github.com/erikkinding/sk/archive/refs/tags/v0.3.9.tar.gz"
  sha256 "3be7d82f966b626c0baecf49209cb004240ba1cb83180fdc8a8c57a0ac537a6d"
  license "MIT"

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X main.version=#{version}"
    system "go", "build", *std_go_args(ldflags:)
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sk --version 2>&1", 1)
  end
end
