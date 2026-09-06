# MCP Host Bridge

Connect AI clients to your existing MCP servers without copying credentials between
machines. Run a small Go gateway on each host, expose explicit capabilities over
Streamable HTTP, or keep using local stdio.

**Beta software.** Local Linux and macOS workflows are tested. Windows users should
use WSL for stdio backends. Web-account integrations and native Windows subprocess
containment are not certified.

## Why use it?

- **Reuse your setup:** read selected definitions from a host-local registry.
- **Share eligible backends:** explicitly attested stateless services can reuse one
  backend per endpoint and authenticated principal.
- **Keep state separate:** session-owned backends remain the default for project
  selectors, interpreters and other stateful tools.
- **Control exposure:** exact tool, prompt and resource allowlists apply to discovery
  and invocation.
- **Keep credentials local:** upstream secrets are resolved on their owning host.

Transport conversion alone does not reduce backend count. Use shared scope only after
checking the backend's state and permission contract. This is not an OS sandbox.

## Try the isolated demo

Requires Go 1.25 or later. The module pins a tested toolchain.

```sh
go build -o bin/mcp-host-bridge ./cmd/mcp-host-bridge
go build -o bin/mcp-bridge-demo ./cmd/mcp-bridge-demo
export MCP_BRIDGE_TOKEN="$(openssl rand -hex 32)"
./bin/mcp-host-bridge check --config examples/bridge.json
./bin/mcp-host-bridge serve --config examples/bridge.json
```

Connect to `http://127.0.0.1:8787/mcp/workstation/demo` with an
`Authorization: Bearer <token>` header. The counter keeps state within a connection.
The demo's hidden tool is not exposed.

For a stdio-only client, launch a lightweight relay to that daemon:

```sh
./bin/mcp-host-bridge relay --config examples/bridge.json --server demo
```

Use `stdio` instead of `relay` to run a private backend directly inside the client's
inherited OS execution boundary. Neither mode attaches to an existing CLI's pipes.

## Documentation

- [Configuration](docs/configuration.md)
- [Ownership and lifecycle](docs/architecture.md)
- [Deployment](docs/deployment.md)
- [Compatibility](docs/compatibility.md)
- [Security](SECURITY.md)

## Development

```sh
go test -race ./...
go vet ./...
sh scripts/build.sh
uv run --with mcp==1.29.1 python scripts/shared_smoke.py --bin-dir dist/linux-amd64
```

Tests use isolated fixtures, temporary credentials and loopback services. They do not
contact real registries, databases, exchanges or notification services. No Docker
daemon is needed. Public examples contain no private deployment inventory.

Built with the official MCP Go SDK. MIT licensed; see [third-party notices](THIRD_PARTY_NOTICES.md).

