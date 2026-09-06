# Security boundaries

Experimental software: review its exposure policy and test in isolation before deployment.
Report suspected vulnerabilities privately through the repository's security reporting
feature when available. Do not publish credentials or private payloads in issues.

The operator and the selected registry entries/executables are trusted. A registry entry
can launch code with the bridge's OS permissions. The gateway is not a sandbox. Keep
policy/registry/private key material owner-controlled and use restricted OS users or
containers for untrusted backends. Backend processes may access the same home, files,
DB and network. Do not expose a trusted local CLI's entire tool surface to the internet.

HTTP requires bearer or externally issued RS256 JWT credentials. JWT checks include
signature, issuer, audience, expiration, optional not-before, subject and scope. The
configured key is a public verification key; OAuth signing/private keys are not needed.
Key rotation requires an explicit configuration/restart; dynamic JWKS is not implemented.
Tokens are never passed through to subprocesses. Registry-configured upstream headers
are distinct credentials and redirects are refused to avoid forwarding them to another URL.

Exact Host/Origin allowlists run before routing. TLS termination is external; the binary
does not infer identity from X-Forwarded headers. Session IDs are bound by the SDK to the
authenticated principal. A shared static token is one principal, not per-user authorization.
OAuth scopes currently apply to the entire configured server set; use separate instances
or policies for different exposure profiles.

HTTP input bodies, backend output frames/bodies and concurrent work are bounded before
decoding. Backend CPU/memory requires OS-level containment. Linux/macOS process groups
are terminated at session close; descendants that deliberately detach into another group
require a cgroup/container boundary. Native Windows stdio spawning is disabled until a
Job Object implementation is available. Long-running tool cancellation is best-effort: cancellation is not a
transaction rollback. No automatic retries are performed for tool execution.

stderr from subprocesses is discarded to avoid credential-bearing diagnostic leakage.
Backend transport errors are replaced with fixed messages. Allowed tool results are
returned as data and may contain sensitive information by design; review the backend's
own data access controls. No request/result logging is enabled by default.

Opt-in shared backends intentionally share backend memory among connections of the same
authenticated principal and configured endpoint. A static bearer represents one principal.
Do not attest stateful tools as stateless: project selection, session variables, pending
transactions, client roots and per-client caches can leak or interfere across connections.
Sharing is not enabled by annotations. Sharing increases the backend crash blast radius;
all attached clients must disconnect after a failed backend before a fresh generation.

MCP tool annotations and local client hooks are not authorization. A web provider never
executes your local PreToolUse hooks. Tool-name allowlists cannot constrain arbitrary
SQL, paths or commands accepted by a selected tool; narrow backend permissions instead.
