# Compatibility

| Capability | Evidence or boundary |
|---|---|
| stdio and Streamable HTTP | Isolated SDK and subprocess integration tests |
| Connection-owned state | Separate counter state across clients |
| Explicit shared state ownership | Multiple clients, one backend; principal isolation |
| Tool/prompt/resource allowlists | Listing and direct invocation checks |
| Cancellation, deadlines, idle and close | Regression tests; no automatic tool retries |
| Body/output caps and process cleanup | Adversarial fixtures and race tests |
| Linux amd64 and macOS ARM64 | CI execution |
| Other Linux/macOS architectures | Portable builds; see release-specific evidence |
| Native Windows stdio | Unsupported pending Job Object containment |
| Windows with WSL | Use Linux binaries |
| Protocol through 2025-11-25 | Supported legacy negotiation |
| Modern sessionless-only protocol | Unsupported |
| Sampling, elicitation, tasks, subscriptions, replay | Not advertised |
| Public web accounts | Require independent endpoint/account verification |

Optional sharing is not blanket backend certification. Measure actual client
context usage and total memory separately from process counts. No benchmark
superiority or universal token-savings percentage is claimed.
