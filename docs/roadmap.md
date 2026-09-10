# Roadmap and validation boundaries

The public beta provides a gateway, opt-in backend sharing and optional compact
metadata. It does not certify every MCP server. Successful connection tests do not
prove safe sharing or token savings.

What the 0.3.0-beta.2 validation round settled:

- Backend sharing is the exception, not the rule. Of the additional integrations
  audited, none qualified: project selectors, graph services with process-global
  state, database clients and vault editors stay session-owned. Sharing remains
  limited to attested stateless read services.
- Compaction pays only on large catalogs. Where descriptions already fit the limit,
  the describe helper is pure overhead; leave `description_limit` at 0 there.
- The effective lever for oversized credentialed servers is an explicit allowlist
  behind session scope, which also removes write tools from reach and moves the
  credential into the gateway-owned registry.

Planned work, subject to implementation and verification:

- Policy reload without a gateway restart, or a documented rolling-restart procedure
  for hosts with long-lived relay sessions.
- Runtime accounting for HTTP-served and containerised backends that the process
  table cannot see.
- Native-client coverage for newer Codex releases and for cancellation, cleanup and
  rollback paths on each host.
- Measure complete client context usage, including deferred loading and on-demand
  guidance, rather than inferring savings from catalog bytes or process counts.

Keep session isolation when the sharing contract is unproven. These plans are not
support guarantees. See [compatibility](compatibility.md), [security](../SECURITY.md)
and [compact metadata](metadata.md) for the current boundaries.
