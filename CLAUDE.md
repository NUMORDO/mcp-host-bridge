# MCP Host Bridge

Go host-local MCP gateway. Read README.md, docs/design.md and docs/research.md.
Never copy host credentials, registries, sessions or private inventory into this repository.
All tests use isolated fixture subprocesses; never start production MCPs or send notifications.
Keep Go source files below 400 lines. Use the official MCP SDK; do not implement JSON-RPC framing.
Required checks: go test -race ./..., go vet ./..., portable build matrix.
Public deployments and web-product compatibility require independent recorded evidence.
