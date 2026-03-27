class Cafaye < Formula
  desc "Non-interactive CLI for agents and operators using Cafaye"
  homepage "https://github.com/cafaye/cafaye-cli"
  url "https://github.com/cafaye/cafaye-cli/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "79556591ec714fd3d0c8cd0cf65534f94dcdaf2c53d25d0fe94d3e016f3ac0fc"
  license "MIT"
  head "https://github.com/cafaye/cafaye-cli.git", branch: "master"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "."
  end

  def post_install
    system "#{bin}/cafaye", "workspace", "init"
  end

  test do
    assert_match "cafaye", shell_output("#{bin}/cafaye --help")
  end
end
