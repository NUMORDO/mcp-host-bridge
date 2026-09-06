# Ownership and lifecycle

- `serve`: authenticated HTTP with lazily started backend sessions.
- `relay`: stdio connected to a local HTTP gateway.
- `stdio`: a private backend in the client's inherited OS execution boundary.

The official MCP SDK handles framing, cancellation and protocol lifecycle. Legacy
protocols through 2025-11-25 are supported. Modern sessionless-only peers are rejected.
A legacy HTTP backend without a session ID is accepted only by an explicitly
attested shared/stateless endpoint. This does not enable the newer protocol.

Each frontend owns its backend process or HTTP session by default. Shared scope
uses a separate pool for each endpoint and authenticated principal. Concurrent first
calls serialize startup. The last frontend closes and reaps the backend. Failed
startup remains sticky until attached frontends disconnect. Tools are not retried.

Call limits, deadlines, idle expiry and input/output caps bound work. Live stdio
relays send SDK heartbeats at one third of the idle timeout. Closing the relay stops
heartbeats; heartbeats never execute tools.

Unix children use a new process group and independently owned pipes. Process
separation protects connection-local memory, not shared files, databases or network
services. Descendants that deliberately detach require OS containment.
