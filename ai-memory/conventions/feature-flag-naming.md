# Convention: Feature Flag Naming

- Format: `{domain}.{feature}` — e.g. `content.page_builder`, `ads.placement`.
- Per-provider flags within a domain: `{domain}.provider.{provider_name}` — e.g. `payments.provider.nowpayments`.
- Default state is explicit at definition time (default ON vs. default OFF) — never leave it implicit.
- A flag controls whether a feature is reachable at all, not fine-grained behavior within an already-enabled feature — fine-grained behavior is a setting, not a flag.
