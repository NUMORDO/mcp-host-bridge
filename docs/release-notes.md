# 0.3.0-beta.1

First clean public beta of MCP Host Bridge.

- Host-local registry resolution and explicit capability allowlists.
- Streamable HTTP, local stdio, and stdio relay transports.
- Session isolation by default; opt-in sharing for attested stateless backends.
- Optional compact descriptions with on-demand original tool guidance.
- Valid private cache scopes for strict MCP clients, including Codex 0.153.4.
- Process cleanup, connection limits, authentication and fixture integration tests.

Native Codex startup and read-only calls were tested for Context7 and SearxNG.
These results cover those integrations and versions only. They do not certify
arbitrary MCP servers, web-account integrations, or native Windows subprocesses.

Compaction changes descriptions, not validation contracts. Byte savings depend on
the catalog and do not establish billable token savings. Shared mode requires an
operator review of backend state, authorization, and filesystem access.
