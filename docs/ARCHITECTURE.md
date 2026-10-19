# Architecture and threat model

```
ChatGPT/MCP client (subject to account permissions)
    -> authorized secure private tunnel
    -> 127.0.0.1:8765/mcp
    -> JSON-RPC Streamable HTTP server
    -> tool dispatch
    -> file operations under configured roots or optional subprocess
```

For local testing, bearer token authorization is supported. ChatGPT's plugin connection UI does not generally accept a raw bearer token. Use a verified Secure MCP Tunnel or future OAuth gateway; no-auth local is ONLY for such a trusted tunnel.

The MCP implementation is deliberately stateless and returns JSON for every RPC request. It supports 2025-03-26, 2025-06-18 and 2025-11-25 initialize versions. SSE subscription and full MCP feature coverage are out of scope. This prototype has not been certified against all MCP clients.

Threat model: an MCP client, prompt-injected content, or a compromised tunnel could request undesired mutations. The system mitigates some risk with loopback binding, optional bearer auth, opt-in terminal, root constraints on file tools, sha256 conditional writes, limits, and subprocess timeout. It does not protect against root-authorized runtime, symlink races or arbitrary behavior of enabled programs. Host ACLs and trusted account-level approvals remain essential. This tool should not be deployed directly to the Internet.
