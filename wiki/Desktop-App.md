# Desktop App

The Virta desktop app is a native window built with [Electron](https://www.electronjs.org). It bundles the `virtad` daemon alongside the app — no separate install, no terminal required. Open the app and it is ready.

---

## Platforms

| OS | Status | Notes |
|---|---|---|
| Windows | Supported | Bundled Chromium (Electron). No external runtime needed. |
| macOS | Supported | Bundled Chromium. `.dmg` installer. |
| Linux (X11) | Supported | AppImage. Bundled Chromium. |
| Linux (Wayland) | Supported | Runs natively or via XWayland. |

Because Electron ships its own Chromium, the renderer is identical on every platform — no per-OS webview quirks.

---

## Download

Grab the latest release from the [Releases](https://github.com/elythi0n/virta/releases) page:

- **Windows**: `Virta-Setup.exe`
- **macOS**: `Virta.dmg`
- **Linux**: `Virta-x86_64.AppImage`

### Linux AppImage

```bash
chmod +x Virta-x86_64.AppImage
./Virta-x86_64.AppImage
```

### macOS

Open the DMG, drag Virta to Applications.

On first run macOS may block the app because it is not signed/notarized yet. Open **System Settings → Privacy & Security** and click **Open Anyway**.

### Windows

Run `Virta-Setup.exe`. The installer lets you choose the install location.

---

## How it works

When you launch the app:

1. The bundled daemon (`virtad`) is started as a child process into an app-private runtime directory.
2. The app waits (up to 20s) for the daemon to advertise its address and pass a health check.
3. A small loopback HTTP server serves the web UI over `http://localhost`, and the UI connects to the daemon automatically.
4. Closing the app terminates the daemon — reliably, including on Windows.

Each launch starts its own daemon; it does not attach to unrelated daemons left over from other tools.

---

## Data location

The daemon stores data in the standard OS locations:

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

The titlebar is draggable — click and drag to move the window. Window edges resize.

---

## Opening streams

Twitch and Kick streams **play inline** in the app — the player iframe is embedded directly in the stream panel. The desktop shell serves the UI from `http://localhost` (so the players accept it as an embed parent) and strips the players' frame-blocking headers, which is what the move to Electron's bundled Chromium made possible.

YouTube has no inline embed, so its stream card opens a pop-out player window (or your browser).

The chat feed, mentions, filter rules, analytics, and all other features run fully inside the app.

---

## Developer tools

Press **F12** or **Ctrl+Shift+I** to open Chromium DevTools in any build (also available from **Settings → About**).

---

## Building from source

Requires Node.js and Go (no Wails CLI, no WebKit/CGO).

```bash
# Stage the web build + daemon into the Electron shell and install its deps
make app

# Launch it (spawns its own daemon)
make app-run
```

Package installers (run on the matching OS):

```bash
make app-win        # Windows: dist/Virta-Setup.exe
make app-dmg        # macOS:   dist/Virta.dmg
make app-appimage   # Linux:   dist/Virta-x86_64.AppImage
```
