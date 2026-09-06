# Roadmap and validation boundaries

The public beta provides a gateway, opt-in backend sharing and optional compact
metadata. It does not certify every MCP server or complete all deployment-specific
optimization. Successful connection tests do not prove safe sharing or token savings.

Planned work, subject to implementation and verification:

- Review project-keyed caches, editor state, database sessions and filesystem rights
  before recommending shared execution for additional integrations.
- Evaluate project/principal pools where one shared process would mix state.
- Extend metadata-compaction validation independently of backend sharing, preserving
  schemas, original guidance and native permission-visible tool names.
- Measure total process memory and complete client context usage, including deferred
  loading and on-demand guidance, rather than inferring savings from process counts.
- Expand native-client, concurrency, cancellation, cleanup and rollback coverage.

Keep session isolation when the sharing contract is unproven. These plans are not
support guarantees. See [compatibility](compatibility.md), [security](../SECURITY.md)
and [compact metadata](metadata.md) for the current boundaries.
