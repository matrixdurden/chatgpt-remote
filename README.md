# ChatGPT Remote

[**Türkçe kurulum rehberi**](docs/TR.md)

**Self-hosted MCP server for accessing project files and running optional commands on your own Linux machine from an MCP-compatible client.**

> Unofficial open-source project. Not affiliated with OpenAI. **Prototype v0.1.0.** The local MCP protocol works without an OpenAI API key, GitHub Actions runner, or Codex. Connecting it to ChatGPT requires a compatible account, an accessible secure tunnel/HTTPS endpoint, and user approval. **A ChatGPT Plus subscription does not guarantee custom MCP write/command access.**

## What works in v0.1.0

- **MCP Streamable HTTP** at `/mcp`; JSON-RPC `initialize`, `ping`, `tools/list`, `tools/call`.
- `list_files`, `read_file`, `search_text` (literal search), `write_file` (sha256 conflict check), `get_status`.
- `run_command` **disabled by default**. When enabled, runs an allowlisted executable directly with an argument array (no implicit shell), 30-second timeout, limited output.
- Configured directory roots, symlink-aware basic path checks, local-only listening, token auth, service status, local protocol check.
- Source installer for Linux / WSL, optional **systemd --user** service.
- Go standard library only; no runtime dependency or OpenAI API billing.

## Platform support

| System | Supported | Notes |
|---|---|---|
| Linux (Ubuntu / Debian, x86-64 or ARM64) | ✅ | Native |
| Windows 11 with WSL2 Ubuntu/Debian | ✅ | Install INSIDE WSL Linux terminal |
| Windows PowerShell or CMD without WSL | ❌ | No native Windows executable yet |

In WSL, Linux files work normally. You can authorize Windows files mounted under /mnt/c by explicitly selecting a root there; this does **not** grant full Windows desktop, GUI or process control. Terminal commands are disabled by default.

## One-command installation (Linux and WSL2)

~~~bash
curl -fsSL https://raw.githubusercontent.com/matrixdurden/chatgpt-remote/main/install.sh | bash
~~~

If you want to share a specific directory instead of the default:

~~~bash
curl -fsSL https://raw.githubusercontent.com/matrixdurden/chatgpt-remote/main/install.sh | bash -s -- --root "$HOME/projects"
~~~

The explicit root must exist already. By default, the installer chooses ~/projects when available, otherwise creates ~/chatgpt-remote-workspace. Only the selected root can be accessed by filesystem MCP tools. Avoid selecting your entire home directory.

**No sudo, git clone, or preinstalled Go required.** Installer downloads checksum-verified release archives if published; otherwise downloads the source archive and compiles it, temporarily downloading a checksum-verified Go toolchain only when necessary. Temporary build artifacts are removed. On Linux with an active user-level systemd service manager, the process starts through systemd; otherwise it starts in the background. In WSL without systemd, the process does not survive WSL shutdown.

### Test locally

~~~bash
~/.local/bin/chatgpt-remote doctor
~/.local/bin/chatgpt-remote status
~~~

The MCP server listens only on http://127.0.0.1:8765/mcp. Passing the local doctor test does not prove that your ChatGPT account can connect remotely.

## One-command complete uninstallation

~~~bash
bash "$HOME/.local/share/chatgpt-remote/uninstall.sh"
~~~

Or, to download the uninstaller:

~~~bash
curl -fsSL https://raw.githubusercontent.com/matrixdurden/chatgpt-remote/main/uninstall.sh | bash
~~~

This stops the managed process or service, deletes the installed binary, private token, config, PID/logs and associated directories. An installer-created workspace is removed only when empty, so user files and your original projects are **never deleted**. Any external tunnel or ChatGPT plugin connection must be revoked separately. This installer does not change shell startup files and does not install Go system-wide.

For source contributors, use git clone followed by ./scripts/install.sh --root /your/project (requires Go 1.23+ when no precompiled binary is bundled).

## Connect to ChatGPT

### IMPORTANT: Account permissions first

Open https://chatgpt.com/plugins and check whether you can select **Add custom MCP server**. On some plans, this capability or write/modify tool permissions may be unavailable. OpenAI's official docs currently describe the setup in [Custom MCP servers](https://developers.openai.com/api/docs/guides/custom-mcp-server) and [Connect and test your plugin](https://developers.openai.com/plugins/deploy/connect-chatgpt). If your account does not expose custom MCP access, installing this server **does not bypass** that restriction.

### Recommended: secure local tunnel

Use an account-supported **Secure MCP Tunnel**, if available:

1. Run `chatgpt-remote serve --tunnel-local`. This binds to **127.0.0.1:8765 only**, with no bearer authentication **for local tunnel traffic**. Only do this when the tunnel is trusted and access-controlled.
2. Configure the tunnel to forward to `http://127.0.0.1:8765/mcp` following the [official Secure MCP Tunnel guide](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels). The tunnel client needs its own separate setup/authorization; it is **not installed by this project**.
3. In ChatGPT Plugins, choose `Add custom MCP server` → `Tunnel` and select the configured tunnel (if your account offers that option), then create and install the plugin.
4. Start a chat supporting the installed plugin, mention it with `@`, and ask `Use ChatGPT Remote get_status, then list_files for <your configured root>`.

**Do not** start the unauthenticated mode and forward it using a public ngrok or Cloudflare HTTPS URL without an independent, working authentication layer. `--tunnel-local` is strictly for a trusted private tunnel. The URL `/health` provides only a local health indication and requires no auth.

### Authenticated HTTP for other MCP clients

Default `serve` requires `Authorization: Bearer <secret>`. This allows local CLI/Inspector tests. ChatGPT's custom MCP wizard generally offers **OAuth or no-auth**, **not a user-supplied static bearer token**. The repository does **not** yet implement OAuth: do not assume that a public HTTPS URL to the token-protected endpoint is enough for ChatGPT. Use a vetted OAuth gateway or supported secure tunnel instead.

**No raw server port should ever be exposed to the public Internet.**

## Local test

```bash
chatgpt-remote doctor
```

Or perform the handshake yourself (do not put the secret in a public screenshot):

```bash
TOKEN="$(cat "$HOME/.config/chatgpt-remote/token")"
curl -sS -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  --data '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"local-test","version":"0.1"}}}' \
  http://127.0.0.1:8765/mcp
unset TOKEN
```

Read [docs/TESTING.md](docs/TESTING.md) for end-to-end MCP tests.

## Enable commands (opt in)

The default `config.json` includes:

```json
{
  "address": "127.0.0.1:8765",
  "roots": ["/home/you/projects"],
  "enable_commands": false,
  "allowed_commands": ["git", "go"]
}
```

To allow command calls, manually set `"enable_commands": true` and restart. Add executable names only after considering their powers. For example, `git`, `go`, or interpreters can themselves execute arbitrary code, access files outside roots, or make network calls. **The root allowlist constrains the MCP file tools and working directory, not what a spawned program can do.** It is not a sandbox. Do not enable commands on machines or projects you cannot trust. Use a dedicated non-root Unix user, restricted filesystem permissions, and preferably a dedicated VM/container for untrusted code. Never add shell interpreters (`bash`, `sh`) to the allowlist without understanding the consequences.

File operations have file-size limits and basic symlink checks but **do not provide race-proof filesystem isolation**. Do not run as root. Always review proposed commands/patches.

## CLI

| Command | Purpose |
|---|---|
| `chatgpt-remote setup --root DIR` | One-time initialization |
| `chatgpt-remote serve` | Start authenticated MCP server |
| `chatgpt-remote serve --tunnel-local` | Start loopback-only server without bearer auth for trusted tunnel |
| `chatgpt-remote doctor` | Test localhost init and tools/list on authenticated server |
| `chatgpt-remote status` | Show endpoint and scope |
| `chatgpt-remote token rotate` | Rotate token (restart required) |
| `./scripts/uninstall.sh` | Remove executable and optional service |

## Project layout

```text
cmd/remote/             CLI entry point
internal/remote/        MCP transport, secure file ops, command execution
scripts/               source installation/uninstallation
.github/workflows/      CI unit tests
 docs/                  testing and architecture notes
```

## Limitations and roadmap

This first version is a tested local prototype, **not** a completed ChatGPT authorization integration. Next steps: real ChatGPT tunnel handshake on an eligible account, automated tunnel setup, OAuth for public HTTPS, Windows service packages, signed releases, audit logging, safer process sandboxing, high-volume file search and streaming task execution. Any automatic ChatGPT-side connection still requires the user's authorization.

## License

MIT. Naming is descriptive only; ChatGPT is an OpenAI trademark. This project is independent and does not imply an official affiliation.