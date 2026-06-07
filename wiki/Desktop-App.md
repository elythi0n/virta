# Desktop App

The Virta desktop app is a native window built with Wails v3. It bundles the `virtad` daemon inside the binary — no separate install, no terminal required. Open the app and it is ready.

---

## Platforms

| OS | Status | Notes |
|---|---|---|
| Linux (X11) | Supported | GTK3 + WebKitGTK. Tested on XFCE/xfwm4. |
| Linux (Wayland) | Partial | Works via XWayland. Native Wayland window is pending. |
| macOS | Supported | WKWebView. Universal binary (arm64 + amd64) in production builds. |
| Windows | Supported | WebView2 (Edge). Requires the Edge WebView2 Runtime (included in Windows 11 and any recent Edge install). |

---

## Download

Grab the latest release from the [Releases](https://github.com/elythi0n/virta/releases) page:

- **Linux**: `virta-linux-amd64.AppImage` or `virta-linux-amd64.tar.gz`
- **macOS**: `Virta.dmg`
- **Windows**: `Virta-Setup.exe` or `Virta-portable.zip`

### Linux AppImage

```bash
chmod +x virta-linux-amd64.AppImage
./virta-linux-amd64.AppImage
```

### macOS

Open the DMG, drag Virta to Applications.

On first run macOS may block the app because it is not from the App Store. Open **System Settings → Privacy & Security** and click **Open Anyway**.

### Windows

Run the installer or extract the portable zip. WebView2 Runtime is required — if it is not installed, the app will prompt you to download it.

---

## How it works

When you launch the app:

1. The daemon (`virtad`) is extracted to your OS cache directory and started as a child process.
2. The app waits up to 15 seconds for the daemon to be ready.
3. The embedded web UI loads and connects to the daemon automatically.
4. Closing the app stops the daemon cleanly.

If a daemon is already running (e.g. from a previous session or a terminal), the app attaches to that instead of launching a new one.

---

## Data location

The desktop app stores data in the standard OS locations:

| OS | Data | Config |
|---|---|---|
| Linux | `~/.local/share/virta/` | `~/.config/virta/` |
| macOS | `~/Library/Application Support/virta/` | same |
| Windows | `%APPDATA%\virta\` | same |

SQLite database: `<data>/virta.db`
Credentials (keychain): OS keychain (macOS Keychain, GNOME Keyring, Windows Credential Manager)

---

## Window controls

The desktop app uses a custom frameless titlebar. Window controls are in the top-right corner:

- **–** minimise
- **□** maximise / restore
- **×** close

The titlebar is draggable — click and drag to move the window. Window edges are draggable to resize.

---

## Opening streams

Twitch and Kick streams cannot play directly inside the app window because WebKitGTK does not support the WebGPU/WebCodecs stack required by the Twitch IVS player. Clicking a stream card shows a **"Watch on Twitch/Kick"** button that opens the stream in your default browser.

The chat feed, mentions, filter rules, and all other features work fully inside the app.

---

## Developer tools (debug build)

To inspect the UI or debug JavaScript:

```bash
make app-debug
./frontends/desktop/build/bin/virta-debug
```

Then: **Settings → About → Open WebKit Inspector**.

The standard `make app` build does not include DevTools.

---

## Building from source

```bash
# Install Wails v3 CLI
go install github.com/wailsapp/wails/v3/cmd/wails3@latest

# Build (Linux with webkit2gtk-4.1)
make app
```

The output is `frontends/desktop/build/bin/virta`.
