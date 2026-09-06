# Configuration

Every server requires an explicit allowlist and exactly one `command` or
`registry_name`. Registry references resolve on the gateway's own host.

```json
{
  "host": "workstation",
  "listen": "127.0.0.1:8787",
  "registry": "/absolute/path/to/private-registry.json",
  "auth": {"token_env": "MCP_BRIDGE_TOKEN"},
  "servers": {
    "search": {
      "registry_name": "search-server",
      "tools": ["search"],
      "backend_scope": "shared",
      "stateless": true
    }
  }
}
```

This is an operator attestation, not proof that an installed server is stateless.
Omit `backend_scope` and `stateless` to retain private session ownership. Registry
command, arguments, working directory, environment and HTTP headers cannot be
overridden by an exposure policy.

`inventory --registry <path>` prints names, transport types and environment/header
key names only. It never starts servers or prints secret values. `check --config <path>`
validates and resolves the policy without starting a backend.

Use native Streamable HTTP when supported to avoid a relay subprocess. The endpoint
is `/mcp/<host>/<server>`. Stdio-only clients can use `relay`; its local environment
must contain the configured bearer variable. Relay refuses non-loopback and
OAuth-only daemon configurations.

Keep configuration owner-readable and outside source control. Each host loads its
own backend registry and credentials.
