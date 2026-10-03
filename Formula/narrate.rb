class Narrate < Formula
  desc "Conversational document narration CLI"
  homepage "https://github.com/narrate-it/narrate"
  version "1.1.0"

  on_macos do
    on_arm do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_darwin_arm64"
      sha256 "3b8a136f62484246ac2cdc49ca0697e5683c214390fbc3d14695bdde828e9b5d"
    end
    on_intel do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_darwin_amd64"
      sha256 "5c45a48287c0b8bcf684321cf9612fa6ef03936067d6742ee984e741704eaafb"
    end
  end
  on_linux do
    on_arm do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_linux_arm64"
      sha256 "6fd198be8694df39eae2c6d23fcf0f887f8c885b94b4bb5ab7e16d8168552254"
    end
    on_intel do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_linux_amd64"
      sha256 "a859ad6ab47bcdd03e7b0c845decae85ef033bdb877a95b50bd6eb90500e26f8"
    end
  end

  def install
    bin.install cached_download => "narrate"
  end

  test do
    assert_match "narrate version #{version}", shell_output("#{bin}/narrate --version")
  end
end
