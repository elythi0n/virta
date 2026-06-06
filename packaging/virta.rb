cask "virta" do
  version "1.0.0"
  sha256 "PLACEHOLDER"

  url "https://github.com/elythi0n/virta/releases/download/v#{version}/Virta-#{version}-macOS-universal.dmg"
  name "Virta"
  desc "Unified live chat for Twitch, Kick, and X"
  homepage "https://virta.lol"

  app "Virta.app"

  uninstall quit: "com.virta.Virta"
  zap trash: [
    "~/Library/Application Support/virta",
    "~/Library/Logs/virta",
  ]
end
