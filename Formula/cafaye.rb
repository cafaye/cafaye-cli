class Cafaye < Formula
  desc "Non-interactive CLI for agents and operators using Cafaye"
  homepage "https://github.com/cafaye/cafaye-cli"
  url "https://github.com/cafaye/cafaye-cli/archive/refs/tags/v0.3.2.tar.gz"
  sha256 "4d5edf74908aa3f7a96589d4c4ff023dcc509b92b20d8e353b47d25e80ee8279"
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
