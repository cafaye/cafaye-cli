class Cafaye < Formula
  desc "Non-interactive CLI for agents and operators using Cafaye"
  homepage "https://github.com/cafaye/cafaye-cli"
  url "https://github.com/cafaye/cafaye-cli/archive/refs/tags/v0.2.12.tar.gz"
  sha256 "0ab54b70012fa8d28a267b61c0f35e1effb80e722562496a2a82832fbcd0f476"
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
