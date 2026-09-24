class Narrate < Formula
  desc "Conversational document narration CLI"
  homepage "https://github.com/narrate-it/narrate"
  version "1.0.0"

  on_macos do
    on_arm do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_darwin_arm64"
      sha256 "52dcb3345be9b15c9b7335c2e72ac10c82369aeaea0a97d4fe95b729f88778a4"
    end
    on_intel do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_darwin_amd64"
      sha256 "4581fa63ca9c5553afe92bc9ed560155196988cff9f5051ca6aefaa920f1eead"
    end
  end
  on_linux do
    on_arm do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_linux_arm64"
      sha256 "494a381cd061d96bde07eab9b9c26f10e574d68d16e3d3d1310ff759c7c84f00"
    end
    on_intel do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_linux_amd64"
      sha256 "a5296d6a17d892edcf02f5fa9b78ec236f24dc714a57a2c2aa566bea93d347f4"
    end
  end

  def install
    bin.install cached_download => "narrate"
  end

  test do
    assert_match "narrate version #{version}", shell_output("#{bin}/narrate --version")
  end
end
