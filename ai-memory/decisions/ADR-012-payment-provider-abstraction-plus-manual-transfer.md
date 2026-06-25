# ADR-012: Payment Provider Abstraction Plus Manual Bank/Mobile Money Transfer

**Status:** Accepted

**Context:** Needed to support Nigerian Naira, international cards, and crypto, plus eventual additional gateways (Apple Pay, Google Pay), and a manual payment path for users without card/online payment access.

**Decision:** Provider abstraction (Go interface) over Paystack, Stripe, NOWPayments, with a fourth `manual_transfer` provider that has no external API — just a generated reference code, proof-of-payment upload, and admin review queue. Apple Pay/Google Pay are not separate providers; they're payment methods inside Stripe/Paystack's existing checkout UI and need no abstraction-layer change.

**Rationale:** The interface-based abstraction means adding a future gateway is a new implementation, not a schema or API breaking change — the `payments` table already stores `provider` as a plain string. Manual transfer needed a fundamentally different shape (human verification instead of a webhook) so it gets its own status (`pending_verification`) rather than being forced into the automated-provider flow.

**Consequences:** Recurring billing (Pro membership) is restricted to Stripe and Paystack — NOWPayments and manual transfer are one-time-purchase only, since neither has a reliable mechanism for automated recurring charges.

**PCI scope, made explicit:** All card collection happens through Stripe Elements or Paystack's hosted checkout. Raw card data (PAN, CVV) never touches platform servers in any code path — payments code only ever handles provider-issued tokens/references. This was previously an implicit assumption baked into the abstraction layer's design; stated outright here so it's an explicit constraint any future provider implementation must honor, not something a new contributor has to infer.

**Addendum (post-audit):** A fifth provider value, `credit`, was added — see Section 17 amendment and ADR-024. Unlike the four providers above, Credit has no external API and isn't a `Provider` implementation at all; it's a direct ledger debit against the Section 28 Wallet & Credits balance. It joins NOWPayments and manual_transfer in the one-time-purchase-only restriction class — no recurring `pro_membership` billing via Credit, for the same reason: no reliable automated recurring-charge mechanism.
