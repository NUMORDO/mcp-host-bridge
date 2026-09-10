# 0.3.0-beta.2

Validation release. The gateway code is unchanged from 0.3.0-beta.1 except for the
reported version string; this release records what was verified on real hosts and
tightens the documentation accordingly.

Verified since 0.3.0-beta.1:

- **Allowlist on a large credentialed backend.** A third-party exchange MCP server
  advertising 382 tools was placed behind the gateway on three hosts with an explicit
  18-tool read-only allowlist and session scope. Every host exposed the same 19 tools
  (18 plus the describe helper), a byte-identical 17,513-byte catalog, listed no order
  or transfer tools, and rejected a hidden write tool with `-32602 "tool not allowed"`.
  The catalog shrank from 455,573 bytes to 17,513 bytes; the upstream credential moved
  out of the client registry into the gateway-owned registry file.
- **Registry switch round trip.** Switching a client registry entry to the relay,
  rolling it back and re-applying it completed on a real host using operator tooling
  that is not part of this package; the relay and gateway behaved as documented at
  every step.
- **Compact metadata, measured.** The describe helper costs 406 bytes per compacted
  server. Servers whose descriptions already fit within the limit gain nothing and
  should keep `description_limit` at 0. Gains are real only for large catalogs.
Boundaries that did not change:

- Backend sharing remained limited to the two attested stateless documentation and
  search services. A project-keyed graph service and a vault editor were audited and
  kept session-owned: one swaps process-global state per call, the other has no
  per-conversation write attribution once shared.
- Policy changes are read at startup only; adding a server requires a gateway restart,
  and relay sessions that span the restart must reconnect.
- Backends served over HTTP by other processes or containers are outside the gateway's
  process accounting.
- Native Codex checks cover 0.153.4; 0.154.0 was not re-tested. Linux ARM runtime,
  native Windows subprocesses and public web-account integrations remain unverified.

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
