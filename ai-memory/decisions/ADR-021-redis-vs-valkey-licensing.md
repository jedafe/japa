# ADR-021: Redis 8.x Licensing vs. Valkey

**Status:** Accepted — Valkey

**Context:** Surfaced during a tech-stack version verification pass. The platform was specified against "Redis 7" without a licensing review. Redis Ltd. changed licensing terms over time: 7.4–7.8 moved to a choice of RSALv2 or SSPLv1 (no longer BSD), and 8.0 onward adds AGPLv3 as a third option in a tri-license model. None of these are the permissive BSD terms under which Redis was originally adopted. The Linux Foundation forked the last BSD-licensed version as **Valkey**, which is wire-protocol-compatible with Redis and acts as a drop-in replacement.

**Decision:** Valkey, latest stable (9.x at time of writing). Used for caching, the WebSocket pub/sub bridge, and Asynq queue backing — none of which depend on any Redis-8-exclusive feature.

**Rationale:** This stopped being a close call once the actual adoption data was checked: AWS ElastiCache, Google Cloud Memorystore, and Oracle have all standardized their managed Redis-protocol offerings on Valkey, it's surpassed 100 million Docker pulls, and Valkey 9.x has caught up to or exceeded Redis 8 on throughput and feature breadth. BSD-3-Clause removes the licensing question entirely rather than requiring a legal judgment call on whether SSPL/AGPL's network-service-triggering terms actually bind a deployment that uses the database purely as an internal dependency rather than reselling it as a hosted service — a real but unnecessary question to have to answer when a technically-equivalent BSD-licensed option exists with no downside for this platform's actual use cases.

**Consequences:** Code should still be written against the standard Redis-protocol command set (which is exactly what the abstraction was already designed to do while this was undecided) — this just removes the ambiguity, it doesn't change anything about how the code is structured. Docker Compose service, environment variable names, and any documentation referring to "Redis" should be updated to "Valkey" for accuracy, though `jc:` key namespacing and the wire protocol itself are unaffected.
