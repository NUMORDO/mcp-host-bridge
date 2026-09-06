# Deployment

Start with the bundled demo and a loopback listener. Prefer a native HTTP client,
or use a stdio relay when required. A service manager can keep one gateway per host.
Never print its environment when inspecting status.

Use an owner-controlled registry and exposure policy. Preserve backend working
directories, environment and authorization. Test separate clients before enabling
shared scope. Keep filesystem interpreters and editors inside the client's OS
boundary unless equivalent server-side restrictions have been established.

Public remote endpoints require HTTPS and exact host/origin allowlists. Bearer tokens
or externally issued RS256 OAuth access tokens are supported. The gateway is a
resource server, not an authorization server: issuer, audience, scope, verification
key, client registration and token issuance remain operator responsibilities.

Validate each intended web product separately. LAN reachability, an HTTP response
or an SDK test does not prove account-level compatibility.

Preserve the current binary and policy before upgrades. Do not restart a gateway
with active clients without a maintenance window. Atomic executable replacement
affects future processes; existing processes retain their loaded code. Record
installed-file hashes and running-process versions separately.

Release archives include checksums and licenses. Native Windows stdio spawning is
disabled; Windows users should run the Linux build inside WSL.
