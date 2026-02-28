# Volumio TUI

A terminal user interface (TUI) for controlling [Volumio](https://volumio.com/) music players, built with Go and the [Charm](https://charm.sh/) stack.

## Features

- **Auto-Discovery**: Automatically finds Volumio devices on your local network using mDNS (Zeroconf).
- **Playback Control**: Play, pause, stop, and toggle playback state.
- **Volume Control**: Increase and decrease volume.
- **Status Display**: Shows current track, artist, album, and playback status.
- **Host Configuration**: Manually set the host via flags, environment variables, or within the UI.

## Installation

### From Source

Ensure you have Go 1.25 or later installed.

```bash
go install github.com/bakito/volumio-tui@latest
```

Or clone the repository and build:

```bash
git clone https://github.com/bakito/volumio-tui.git
cd volumio-tui
go build -o volumio-tui .
```

## Usage

Simply run the application:

```bash
volumio-tui
```

The application will attempt to auto-discover a Volumio instance on your network. If multiple instances are found, it will connect to the first one available. If discovery fails, you can manually specify the host.

### Flags

- `-h <host>`: Specify the Volumio host (e.g., `192.168.1.50`, `volumio.local`).
- `-v`: Print version information.

### Environment Variables

You can also set the host using environment variables:

- `VOLUMIO_HOST`
- `VOLUMIO_URL`

Example:

```bash
VOLUMIO_HOST=192.168.1.50 volumio-tui
```

## Key Bindings

| Key | Action |
| :--- | :--- |
| `Space` | Toggle Play/Pause |
| `p` | Play |
| `a` | Pause |
| `s` | Stop |
| `r` | Refresh state |
| `↑` | Volume Up (+5) |
| `↓` | Volume Down (-5) |
| `e` | Edit Host URL |
| `Enter` | Save Host (Edit Mode) |
| `Esc` | Cancel Edit |
| `?` | Toggle Help |
| `q` / `Ctrl+c` | Quit |
