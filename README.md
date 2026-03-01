# LLMPool

**LLMPool** combines your multiple LLM API accounts (OpenAI/ChatGPT keys, OAuth, or future providers) into **one unified, load-balanced endpoint** — bypassing rate limits and maximizing quota while keeping full traceability.

Perfect for heavy users of coding tools (Cursor, Continue.dev, VS Code extensions, aider, etc.) that rely on a single OpenAI-compatible API.

### Key Features

- **LLM-Optimized Load Balancing** — Round-robin, weighted, least-response-time, cost-aware routing — tuned for streaming responses, long contexts, and persistent connections.
- **Append-Only Audit Trail & Logging** — Full request/response history with timestamps, account attribution, tokens used, and immutable logs — critical for debugging, compliance, and cost tracking.
- **Enhanced Observability** — Rich Prometheus metrics out-of-the-box: per-account usage, latency histograms, error rates, queue depth, routing decisions — easy to plug into Grafana or any monitoring stack.
- Open-source core (self-host with Docker) + optional subscription for managed hosting, auto OAuth refresh, advanced routing rules, and priority support.

One endpoint. Zero rate-limit anxiety. Complete visibility.
