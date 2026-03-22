<div align="center">

<img src="docs/ccmux_banner.png" alt="ccmux" width="600">

**Reliable remote control for Claude Code over Telegram**

*Respond to approvals, monitor progress, and stay in control — from any device*

<p>
  <img src="https://github.com/ccmux/ccmux/actions/workflows/pr.yml/badge.svg" alt="CI">
  <img src="https://img.shields.io/github/v/release/ccmux/ccmux" alt="Latest release">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="License">
</p>

</div>

---

Run Claude Code on a server and stay in control from anywhere. Start sessions, send instructions, and handle approval requests — all from Telegram, on any device.

No more being stuck at your desk waiting for Claude to ask permission. No SSH client, no terminal app. Long-running tasks keep going while you're away, and you jump back in whenever you need to. Run multiple sessions in parallel, one per topic, each fully isolated.

## Quick Start

**1. Install**

```bash
curl -sSL ccmux.github.io/install.sh | sh
```

Or download a binary from the [releases page](https://github.com/ccmux/ccmux/releases).

**2. Create a Telegram bot and group**

- Create a bot via [@BotFather](https://t.me/BotFather) and copy the token
- Create a Telegram supergroup and enable **Topics** in group settings
- Add the bot as an admin
- Get your user ID from [@userinfobot](https://t.me/userinfobot) and the group ID from [@RawDataBot](https://t.me/RawDataBot)

**3. Configure**

Create `~/.ccmux/.env`:

```env
TELEGRAM_BOT_TOKEN=your-bot-token
ALLOWED_USERS=your-telegram-user-id
TELEGRAM_GROUP_ID=your-group-chat-id
```

**4. Install the hook**

```bash
ccmux hook --install
```

Registers a hook in `~/.claude/settings.json` so ccmux can link Claude sessions to topics automatically.

**5. Run**

```bash
ccmux
```

Or as a persistent background service — see [Systemd](#systemd).

## Usage

Create a topic in your Telegram group and send:

```
/new /path/to/project     Start a Claude session in that directory
/new                      Start in your home directory
```

Then just type. Your messages go to Claude, responses come back to the topic.

```
/kill                     End the session bound to this topic
/sessions                 List all active sessions
/esc                      Send Escape to Claude
/screenshot               Capture the current terminal state as text
```

### Interactive approvals

When Claude asks for permission to run a command, an inline keyboard appears directly in the chat:

| Button | Action |
|--------|--------|
| ✅ Allow | Allow this once |
| 🔒 Always | Always allow this tool |
| ❌ Deny | Deny |

For interactive prompts (file pickers, menus), navigation buttons appear: ↑ ↓ Enter Esc Space Tab.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `TELEGRAM_BOT_TOKEN` | — | **Required.** Bot token from @BotFather |
| `ALLOWED_USERS` | — | **Required.** Comma-separated Telegram user IDs |
| `TELEGRAM_GROUP_ID` | — | **Required.** Supergroup chat ID |
| `TMUX_SESSION_NAME` | `ccmux` | tmux session name |
| `CLAUDE_COMMAND` | `claude` | Claude binary name or path |
| `CCMUX_DIR` | `~/.ccmux` | State and config directory |
| `POLL_INTERVAL` | `2s` | Response poll interval |
| `CCMUX_QUIET_MODE` | `false` | Suppress tool call messages |
| `CCMUX_SHOW_TOOL_CALLS` | `true` | Show tool use and result pairs |

## Systemd

```ini
# /etc/systemd/system/ccmux.service
[Unit]
Description=ccmux Telegram Claude Code bridge
After=network.target

[Service]
ExecStart=/usr/local/bin/ccmux
Restart=on-failure
RestartSec=5
EnvironmentFile=/root/.ccmux/.env
User=root

[Install]
WantedBy=multi-user.target
```

```bash
systemctl enable --now ccmux
journalctl -fu ccmux
```

## Contributing

Bug fixes and clear improvements are welcome. Open an issue first for anything non-trivial.

## License

MIT
