class Cafaye < Formula
  desc "Non-interactive CLI for agents and operators using Cafaye"
  homepage "https://github.com/cafaye/cafaye-cli"
  url "https://github.com/cafaye/cafaye-cli/archive/refs/tags/v0.2.9.tar.gz"
  sha256 "59d9d66b57c18d11ce81d993547efbd9c908860354857c99395c91125c6be002"
  license "MIT"
  head "https://github.com/cafaye/cafaye-cli.git", branch: "master"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "."
  end

  test do
    assert_match "cafaye", shell_output("#{bin}/cafaye --help")
  end
end
