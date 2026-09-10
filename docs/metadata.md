# Compact tool metadata

Set `description_limit` to 64..4096 on a server to shorten tool descriptions and
schema descriptions in `tools/list`. The default is zero, preserving full metadata.

```json
"search": {
  "registry_name": "search-server",
  "tools": ["search"],
  "description_limit": 256
}
```

Compacted endpoints add `bridge_describe_tool`. Call it with `tool_name` to retrieve
the complete original guidance for an allowed tool. It only reads metadata and
never invokes that tool. Real calls retain their original tool names, so native
client permission rules can continue to identify the actual operation.

Names, annotations, validation keywords, enum/const/default values and parameter
structure are preserved. Only descriptions at schema-valued locations are shortened.
Do not rely on shortened prose as a security control: enforce permissions at the
backend and retrieve full guidance before using an unfamiliar tool.

Measured behaviour (0.3.0-beta.2): the `bridge_describe_tool` helper adds 406 bytes to
every compacted listing. A server whose descriptions already fit within the limit gains
nothing and pays that overhead, so keep `description_limit` at 0 for small catalogs.
On a 382-tool catalog the same limit removed about 27% of the bytes; an explicit
18-tool allowlist removed 94%. Allowlists, not compaction, are the lever for oversized
servers.

Metadata-byte or tokenizer estimates are not billing measurements. Compaction helps
when a large catalog is loaded but only a small subset is used; fetching every full
description can remove the benefit. Prefer native client deferred tool loading
when available, and measure the complete workflow before claiming token savings.
