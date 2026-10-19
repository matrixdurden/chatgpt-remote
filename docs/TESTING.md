# Testing guide

## Automated tests

```bash
go test -race ./...
go vet ./...
go build -o ./bin/chatgpt-remote ./cmd/remote
```

Tests exercise token rejection, MCP handshake/tool listing, disabled terminal command, read/write optimistic concurrency (sha256), symlink escape rejection, loopback-only restrictions and browser Origin rejection.

## Manual local setup

```bash
mkdir -p /tmp/chatgpt-remote-demo
./bin/chatgpt-remote setup --root /tmp/chatgpt-remote-demo
./bin/chatgpt-remote serve
```

Second shell:

```bash
./bin/chatgpt-remote doctor
```

## Manual MCP call

First get tools from `tools/list`. Call `write_file` with `create=true` for a new file; then call `read_file` and capture the `sha256`; update the file using `write_file`, `create=false` and the same `expected_sha256`. A stale digest must return `isError=true`.

Check that `run_command` returns `isError=true` when `enable_commands=false`. Check that paths outside the configured `roots` are rejected.

## ChatGPT integration smoke test

1. Verify the ChatGPT account offers **Add custom MCP server** and use the official Secure MCP Tunnel client if available.
2. Run the local service with `serve --tunnel-local` only behind that authorized private tunnel.
3. Register it as a plugin, install it and invoke `get_status` and `list_files`.
4. Try creating a disposable text file inside the allowed test project after reviewing/approving the change.
5. Verify the file exists and the logs/results match. Do **not** test using sensitive files or production infrastructure.

No end-to-end ChatGPT account connection has been performed by the repository authoring session.
