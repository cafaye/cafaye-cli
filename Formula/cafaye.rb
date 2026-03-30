class Cafaye < Formula
  desc "Non-interactive CLI for agents and operators using Cafaye"
  homepage "https://github.com/cafaye/cafaye-cli"
  url "https://github.com/cafaye/cafaye-cli/archive/refs/tags/v0.3.1.tar.gz"
  sha256 "87c3b22c178f490d3acb44d3ff8bda4a44189f95c0b74dc21fac8f8220ddcd5e"
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
