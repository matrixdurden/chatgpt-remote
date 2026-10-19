# ChatGPT Remote

Self-hosted MCP tools for Linux and Windows 11 **via WSL2**. Unofficial open-source project; not affiliated with OpenAI.

## Install — one command

Open Ubuntu/Linux (or Ubuntu inside WSL) and run:

```bash
curl -fsSL https://raw.githubusercontent.com/matrixdurden/chatgpt-remote/main/install.sh | bash
```

The installer downloads a checksum-verified release, creates a private token, starts the local MCP daemon, and runs its self-test. No `sudo`, system Go install or Docker is required. The default accessible project directory is `~/projects` if it exists, or `~/chatgpt-remote-workspace` otherwise. To choose an existing directory, append `-s -- --root "$HOME/myproject"` to the command after `bash`.

## Connect to ChatGPT — one command

After installation, run:

```bash
chatgpt-remote
```

If the command is not yet on your shell PATH (common in a newly opened WSL session), use `~/.local/bin/chatgpt-remote` or restart the terminal.

The program starts a **temporary Cloudflare Quick Tunnel**, automatically downloads the official checksum-verified `cloudflared` binary on first use, and shows:

- an **HTTPS MCP URL** ending in `/mcp`;
- a **pairing code** (never include this code in the URL).

In [ChatGPT Plugins](https://chatgpt.com/plugins), choose **Add custom MCP server**. Paste the **full HTTPS MCP URL**, select **OAuth / automatic client registration (DCR)** when prompted, create the plugin, and authorize it. A browser page asks for the pairing code shown in your terminal. Then select the plugin in a supported ChatGPT chat and request `get_status` or `list_files`.

**Keep the command running.** Press `Ctrl+C` to disconnect. Cloudflare Quick Tunnels are for **development/testing only**: the temporary URL changes each time you restart the tunnel, so ChatGPT may need a new connection. A Cloudflare account or a domain is **not required** for this mode. For permanent, production-grade connectivity, use a persistent authenticated hostname and a maintained OAuth service.

### Supported capabilities

- Read/list/search files within configured project roots.
- Create or modify text files (`write_file`) with conflict checks.
- `get_status` reports roots and enabled tools.
- Commands are **disabled by default**. They are opt-in via `~/.config/chatgpt-remote/config.json` and are **not sandboxed**, even with the executable allowlist. Do not enable them on an untrusted machine or as root.
- MCP Streamable HTTP at local `127.0.0.1:8765/mcp`, authenticated with a private token.

### Security

Cloudflare Quick Tunnel is not an authentication layer. The application adds an **OAuth 2.1 authorization-code + PKCE** bridge with dynamic client registration, ChatGPT redirect validation, a fresh pairing code, resource-bound access tokens, short-lived authorization codes, and no externally exposed unauthenticated terminal. The public bridge is bound to a private local daemon and requires authorized tokens before forwarding commands. Access tokens and pairing approvals live in memory and disappear on disconnect. This is a **new experimental OAuth implementation**, not yet externally security audited or tested against a real ChatGPT account. Do not use it for production servers or sensitive directories before independent review and end-to-end validation.

ChatGPT's ability to connect to a custom MCP with write permissions depends on your account/plan and interface. A local working server does **not** bypass any plan restriction. OAuth consent is **not** optional even when Cloudflare gives you an HTTPS URL.

### Local checks (only when troubleshooting)

```bash
~/.local/bin/chatgpt-remote doctor
~/.local/bin/chatgpt-remote status
```

### Completely uninstall — one command

```bash
curl -fsSL https://raw.githubusercontent.com/matrixdurden/chatgpt-remote/main/uninstall.sh | bash
```

Stops and removes the service, binary, installed tunnel helper, local token, application settings, PID/log files. Leaves user projects untouched; only deletes an installer-created workspace if **empty**. Close any active `chatgpt-remote` connection before uninstalling, and remove the ChatGPT plugin separately. No shell profiles are edited.

## Platforms

| Platform | Support |
|---|---|
| Linux x86-64, ARM64 (Ubuntu/Debian) | Yes |
| Windows 11 via WSL2 Ubuntu/Debian | Yes, within WSL Linux environment |
| Windows PowerShell/CMD without WSL | Not currently |

For architectural and testing details see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md), [docs/TESTING.md](docs/TESTING.md), and the [Türkçe rehber](docs/TR.md).

**MIT License.** ChatGPT is a trademark of OpenAI. This project does not imply official endorsement.
