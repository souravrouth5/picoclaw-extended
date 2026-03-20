<div align="center">

  <h1>PicoClaw Extended: Zero-Config AI Assistant</h1>

  <h3>One API Key · Auto Free Models · $10 Hardware · &lt;10MB RAM</h3>
  <p>
    <img src="https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/Arch-x86__64%2C%20ARM64%2C%20MIPS%2C%20RISC--V-blue" alt="Hardware">
    <img src="https://img.shields.io/badge/license-MIT-green" alt="License">
    <br>
    <a href="https://github.com/souravrouth5/picoclaw-extended/releases"><img src="https://img.shields.io/badge/Releases-Download-blue?style=flat&logo=github&logoColor=white" alt="Releases"></a>
    <a href="https://github.com/souravrouth5/picoclaw-extended/issues"><img src="https://img.shields.io/badge/Issues-Report-red?style=flat&logo=github&logoColor=white" alt="Issues"></a>
  </p>

</div>

---

> **PicoClaw Extended** is a fork of [PicoClaw](https://github.com/sipeed/picoclaw) with one key addition: **zero-config model management**. Set your OpenRouter API key once — the assistant automatically fetches the best free models, ranks them by capability, and sets up fallbacks. No manual `model_list` editing ever needed.

## ✨ What's Different from PicoClaw

🆓 **Auto Free Models**: Set one OpenRouter key → automatically fetches all free models from OpenRouter, picks the best coding/large-context model as default, wires fallbacks.

🧠 **Smart Model Ranking**: Free models are classified into tiers:
- **Tier 3 (Large Context Coding)** — coding models with 64K+ context (e.g. `deepseek-coder`, `qwen-coder` with 128K context)
- **Tier 2 (Coding)** — coding-focused models under 64K context
- **Tier 1 (General)** — general purpose models, sorted by context length

The best available model is always used first, with automatic fallback through the ranked list if it fails or becomes paid.

🔄 **Paid Model Detection**: If a free model starts charging, it's automatically removed from the pool at runtime — no config changes needed.

Everything else (channels, tools, MCP, skills) is identical to upstream PicoClaw.

---

## 📦 Install

<<<<<<< HEAD
### Linux / macOS / Termux (Android)
### Download from picoclaw.io (Recommended)

Visit **[picoclaw.io](https://picoclaw.io)** — the official website auto-detects your platform and provides one-click download. No need to manually pick an architecture.

### Download precompiled binary

Alternatively, download the binary for your platform from the [GitHub Releases](https://github.com/sipeed/picoclaw/releases) page.

### Build from source (for development)
=======
### Linux x86_64 (most desktops/servers)
>>>>>>> 6f79069 (docs: update README with platform config commands and channel setup guides)

```bash
wget https://github.com/souravrouth5/picoclaw-extended/releases/latest/download/picoclaw-linux-amd64.tar.gz
tar xzf picoclaw-linux-amd64.tar.gz
mv picoclaw-linux-amd64 picoclaw
./picoclaw onboard
```

### Linux ARM64 (Raspberry Pi, etc.)

```bash
wget https://github.com/souravrouth5/picoclaw-extended/releases/latest/download/picoclaw-linux-arm64.tar.gz
tar xzf picoclaw-linux-arm64.tar.gz
mv picoclaw-linux-arm64 picoclaw
./picoclaw onboard
```

### macOS ARM64 (Apple Silicon)

```bash
wget https://github.com/souravrouth5/picoclaw-extended/releases/latest/download/picoclaw-darwin-arm64.tar.gz
tar xzf picoclaw-darwin-arm64.tar.gz
mv picoclaw-darwin-arm64 picoclaw
./picoclaw onboard
```

### Windows

Download `picoclaw-windows-amd64.zip` from the [Releases](https://github.com/souravrouth5/picoclaw-extended/releases/latest) page, extract, and run:

```cmd
picoclaw-windows-amd64.exe onboard
```

### Termux (Android)

```bash
wget https://github.com/souravrouth5/picoclaw-extended/releases/latest/download/picoclaw-linux-arm64.tar.gz
tar xzf picoclaw-linux-arm64.tar.gz
pkg install proot
termux-chroot ./picoclaw-linux-arm64 onboard
```

---

## 🚀 Quick Start (Zero-Config with OpenRouter)

**1. Run onboard**

```bash
./picoclaw onboard
```

**2. Open the config file and add your OpenRouter API key**

The `providers` section is already there after onboard — just fill in your key:

```json
{
  "providers": {
    "openrouter": {
      "api_key": "sk-or-your-key-here"
    }
  }
}
```

Open the config file with:

```bash
# Linux / macOS
nano ~/.picoclaw/config.json

# Termux (Android)
nano ~/.picoclaw/config.json

# Windows (Command Prompt)
notepad %USERPROFILE%\.picoclaw\config.json

# Windows (PowerShell)
notepad $env:USERPROFILE\.picoclaw\config.json
```

Get a free key at [openrouter.ai/keys](https://openrouter.ai/keys) — no credit card needed for free models.

**3. Chat**

```bash
./picoclaw agent -m "Hello!"
```

That's it. On first run, the assistant automatically:
1. Fetches all free models from OpenRouter
2. Ranks them by capability (large-context coding models first)
3. Sets the best one as default with the rest as fallbacks
4. Starts chatting

> **Want a specific paid model instead?** Add it to `model_list` in config and set `agents.defaults.model_name` — the auto-bootstrap skips if you already have a model configured.

---

## 🧠 Model Categorization

When you run with an OpenRouter key, free models are automatically ranked:

| Tier | Type | Example Models |
|------|------|----------------|
| **3** | Large-context coding (64K+) | `deepseek/deepseek-coder`, `qwen/qwen-2.5-coder-32b` |
| **2** | Coding focused | `starcoder`, `codestral` variants |
| **1** | General purpose | Sorted by context length, largest first |

The top-ranked model becomes `or-free-default` and is used automatically. All others are wired as fallbacks in order — if the primary fails, the next best takes over instantly.

To see which models were selected, run:

```bash
./picoclaw status
```

---

## 💬 Chat Channels

Connect the assistant to messaging apps by running `./picoclaw gateway` after configuring a channel below.

| Channel | Difficulty | Notes |
|---------|-----------|-------|
| **Telegram** | Easy | Just a bot token |
| **Discord** | Easy | Bot token + enable intents |
| **WhatsApp** | Easy | QR scan on first run |
| **Matrix** | Medium | Homeserver + access token |
| **LINE** | Medium | Needs HTTPS webhook (use ngrok) |
| **DingTalk** | Medium | App credentials |
| **WeCom AI Bot** | Medium | Bot ID + Secret |

<details>
<summary><b>Telegram</b> (Recommended)</summary>

**1. Create a bot**
- Open Telegram, search `@BotFather`
- Send `/newbot`, follow prompts, copy the token

**2. Get your user ID** from `@userinfobot` on Telegram

**3. Open config and add the telegram section**

```bash
# Linux / macOS / Termux
nano ~/.picoclaw/config.json

# Windows (Command Prompt)
notepad %USERPROFILE%\.picoclaw\config.json

# Windows (PowerShell)
notepad $env:USERPROFILE\.picoclaw\config.json
```

Add or update the `channels.telegram` block:

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "YOUR_BOT_TOKEN",
      "allow_from": ["YOUR_USER_ID"]
    }
  }
}
```

**4. Start gateway**

```bash
# Linux / macOS / Termux
./picoclaw gateway

# Windows
picoclaw-windows-amd64.exe gateway
```

</details>

<details>
<summary><b>Discord</b></summary>

**1. Create a bot**
- Go to https://discord.com/developers/applications
- Create application → Bot → Add Bot → copy token

**2. Enable intents**
- Bot settings → enable **MESSAGE CONTENT INTENT**

**3. Get your User ID**
- Discord Settings → Advanced → enable Developer Mode
- Right-click your avatar → Copy User ID

**4. Open config and add the discord section**

```bash
# Linux / macOS / Termux
nano ~/.picoclaw/config.json

# Windows (Command Prompt)
notepad %USERPROFILE%\.picoclaw\config.json

# Windows (PowerShell)
notepad $env:USERPROFILE\.picoclaw\config.json
```

```json
{
  "channels": {
    "discord": {
      "enabled": true,
      "token": "YOUR_BOT_TOKEN",
      "allow_from": ["YOUR_USER_ID"]
    }
  }
}
```

**5. Invite the bot** via OAuth2 → URL Generator → Scopes: `bot` → Permissions: `Send Messages`, `Read Message History`

**6. Start gateway**

```bash
# Linux / macOS / Termux
./picoclaw gateway

# Windows
picoclaw-windows-amd64.exe gateway
```

> To respond only when @mentioned: add `"group_trigger": { "mention_only": true }` to the discord config.

</details>

<details>
<summary><b>WhatsApp</b></summary>

**1. Open config and add the whatsapp section**

```bash
# Linux / macOS / Termux
nano ~/.picoclaw/config.json

# Windows (Command Prompt)
notepad %USERPROFILE%\.picoclaw\config.json

# Windows (PowerShell)
notepad $env:USERPROFILE\.picoclaw\config.json
```

```json
{
  "channels": {
    "whatsapp": {
      "enabled": true,
      "use_native": true,
      "allow_from": []
    }
  }
}
```

**2. Start gateway**

```bash
# Linux / macOS / Termux
./picoclaw gateway

# Windows
picoclaw-windows-amd64.exe gateway
```

On first run a QR code prints in the terminal. Scan it with WhatsApp → Linked Devices. Session is saved automatically after that.

</details>

<details>
<summary><b>Matrix</b></summary>

**1.** Create a bot account on any homeserver (e.g. matrix.org) and get its access token

**2. Open config and add the matrix section**

```bash
# Linux / macOS / Termux
nano ~/.picoclaw/config.json

# Windows (Command Prompt)
notepad %USERPROFILE%\.picoclaw\config.json

# Windows (PowerShell)
notepad $env:USERPROFILE\.picoclaw\config.json
```

```json
{
  "channels": {
    "matrix": {
      "enabled": true,
      "homeserver": "https://matrix.org",
      "user_id": "@your-bot:matrix.org",
      "access_token": "YOUR_ACCESS_TOKEN",
      "allow_from": []
    }
  }
}
```

**3. Start gateway**

```bash
# Linux / macOS / Termux
./picoclaw gateway

# Windows
picoclaw-windows-amd64.exe gateway
```

</details>

<details>
<summary><b>LINE</b></summary>

**1.** Go to [LINE Developers Console](https://developers.line.biz/) → Create Messaging API channel → copy Channel Secret and Channel Access Token

**2. Open config and add the line section**

```bash
# Linux / macOS / Termux
nano ~/.picoclaw/config.json

# Windows (Command Prompt)
notepad %USERPROFILE%\.picoclaw\config.json

# Windows (PowerShell)
notepad $env:USERPROFILE\.picoclaw\config.json
```

```json
{
  "channels": {
    "line": {
      "enabled": true,
      "channel_secret": "YOUR_CHANNEL_SECRET",
      "channel_access_token": "YOUR_CHANNEL_ACCESS_TOKEN",
      "webhook_path": "/webhook/line",
      "allow_from": []
    }
  }
}
```

**3.** LINE requires HTTPS. Use ngrok to expose the gateway:

```bash
ngrok http 18790
```

Set webhook URL in LINE console to `https://your-ngrok-url/webhook/line`

**4. Start gateway**

```bash
# Linux / macOS / Termux
./picoclaw gateway

# Windows
picoclaw-windows-amd64.exe gateway
```

</details>

<details>
<summary><b>DingTalk</b></summary>

**1.** Go to [DingTalk Open Platform](https://open.dingtalk.com/) → Create internal app → copy Client ID and Client Secret

**2. Open config and add the dingtalk section**

```bash
# Linux / macOS / Termux
nano ~/.picoclaw/config.json

# Windows (Command Prompt)
notepad %USERPROFILE%\.picoclaw\config.json

# Windows (PowerShell)
notepad $env:USERPROFILE\.picoclaw\config.json
```

```json
{
  "channels": {
    "dingtalk": {
      "enabled": true,
      "client_id": "YOUR_CLIENT_ID",
      "client_secret": "YOUR_CLIENT_SECRET",
      "allow_from": []
    }
  }
}
```

**3. Start gateway**

```bash
# Linux / macOS / Termux
./picoclaw gateway

# Windows
picoclaw-windows-amd64.exe gateway
```

</details>

<details>
<summary><b>WeCom AI Bot</b></summary>

**1.** Go to WeCom Admin Console → AI Bot → Create new AI Bot → copy Bot ID and Secret

**2. Open config and add the wecom_aibot section**

```bash
# Linux / macOS / Termux
nano ~/.picoclaw/config.json

# Windows (Command Prompt)
notepad %USERPROFILE%\.picoclaw\config.json

# Windows (PowerShell)
notepad $env:USERPROFILE\.picoclaw\config.json
```

```json
{
  "channels": {
    "wecom_aibot": {
      "enabled": true,
      "bot_id": "YOUR_BOT_ID",
      "secret": "YOUR_SECRET",
      "allow_from": [],
      "welcome_message": "Hello! How can I help you?"
    }
  }
}
```

**3. Start gateway**

```bash
# Linux / macOS / Termux
./picoclaw gateway

# Windows
picoclaw-windows-amd64.exe gateway
```

</details>

---

## 🖥️ CLI Reference

| Command | Description |
|---------|-------------|
| `picoclaw onboard` | Initialize config & workspace |
| `picoclaw agent -m "..."` | One-shot chat |
| `picoclaw agent` | Interactive chat mode |
| `picoclaw gateway` | Start gateway (for chat channels) |
| `picoclaw status` | Show status and active models |
| `picoclaw version` | Show version info |
| `picoclaw cron list` | List scheduled jobs |
| `picoclaw cron add ...` | Add a scheduled job |
| `picoclaw skills list` | List installed skills |
| `picoclaw skills install` | Install a skill |

---

## 🔧 Manual Model Configuration

If you want to use a specific paid model instead of auto free models, add it to `model_list` and set `model_name`:

```json
{
  "agents": {
    "defaults": {
      "model_name": "my-model"
    }
  },
  "model_list": [
    {
      "model_name": "my-model",
      "model": "openai/gpt-4o",
      "api_key": "sk-your-key"
    }
  ]
}
```

The auto-bootstrap is skipped when `model_name` is set and resolves to a valid entry.

---

## 🤝 Credits

Built on top of [PicoClaw](https://github.com/sipeed/picoclaw) by [Sipeed](https://sipeed.com). This fork adds zero-config OpenRouter model management.
