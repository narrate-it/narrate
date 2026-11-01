class Narrate < Formula
  desc "Conversational document narration CLI"
  homepage "https://github.com/narrate-it/narrate"
  version "1.2.1"

  on_macos do
    on_arm do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_darwin_arm64"
      sha256 "d6cf766e548a0824ab7051b78fbd4e881df54156fee74a8437c10d7c103f9f22"
    end
    on_intel do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_darwin_amd64"
      sha256 "c270540aba27afb0d8b9da1166a267246f460f250f4a57d0ea99110139d9b5da"
    end
  end
  on_linux do
    on_arm do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_linux_arm64"
      sha256 "9395e23938de85f0f483c56c228513da1a2db681269a5a945e7cf6ec8489e5b9"
    end
    on_intel do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_linux_amd64"
      sha256 "70c7cf008cee6f82bfb59d4a8e70c9169a8abe2897a167fd1b58ad2479ee1175"
    end
  end

  def install
    bin.install cached_download => "narrate"
  end

  test do
    assert_match "narrate version #{version}", shell_output("#{bin}/narrate --version")
  end
end
