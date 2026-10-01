# JapaConnect Partnership Strategy

Recommended ways for JapaConnect to partner with established agents, agencies, and organizations — mapped to the partnership machinery already specified in JEOS.

## 1. Verified presence (the foundation)

Every partnership should start with an **Organization Profile** (§07). Orgs get a public page at `/org/[slug]`, team members with owner/admin/editor roles, authorship switching (ADR-062 — post *as* the org), follow/follower mechanics, and analytics.

- `org_type` already covers `business | ngo | government | embassy | university | media | community` — so embassies, universities, diaspora NGOs, and media outlets are all first-class citizens, not just commercial agencies.
- **Claim Ownership** (§07) is the onboarding wedge: seed listings for known agencies/orgs yourself, then invite them to claim their listing via the Form Builder flow (evidence URL, business registration, role). Claims can be free or fee-gated via `submission_fee_credits`.
- **Verification badges** (`verified` + `verification_type` = `embassy | business | notable`) are admin-granted, deliberately decoupled from claiming — a claimed-but-unverified org is a normal state. This gives a two-tier trust ladder.

## 2. Service Directory listings (§98)

This is the primary commercial channel for agents and agencies — immigration lawyers, relocation agents, financial advisors, tutors, etc.:

- Geo-indexed by destination country/city + `origin_communities` (e.g. a Lagos-based visa agent serving UK-bound clients).
- `verification_type` tiers: `self | community | admin | professional_body` — differentiates a self-listed agent from a body-accredited one (e.g. an IAA-regulated adviser or Canadian RCIC).
- Integrated with §29 ratings/reviews and the claim flow (`claim_status`, `claimed_by_org_id`).
- v2 links out via `booking_url`; v3 adds a full booking pack with deposits via §17 payments — which becomes a real revenue-share partnership surface.

## 3. Commercial partnership tracks

Three distinct money flows, all already specced:

| Model | Mechanism | Spec |
|---|---|---|
| JapaConnect earns commission from partners | **External Affiliate Providers** — register the partner, inject tracking params at render time, embed coupons | §55 |
| Partners/agents earn from referring users to JapaConnect | **User Affiliate Program** — 30-day attribution, Credits-only payout (locked, ADR-052) | §56 |
| Partners pay for reach | **Org boosts** (featured listings) + **self-serve ads** — restricted to verified orgs only, admin review gate | §17, §25, §61 |

Practical recommendation: for *established* agencies, the coupon/affiliate route (§55) is the cleanest first deal — e.g. "10% off attestation service for JapaConnect members" — because tracking is external and there's no payout infrastructure needed on JapaConnect's side. The self-serve ads gate (`organization_not_verified` → 403) means paid promotion is only available after verification — a good anti-scam forcing function.

## 4. Content & authority partnerships

For organizations whose value is credibility rather than transactions:

- **Ask an Expat AMA (§88)** — scheduled structured AMAs for immigration lawyers/consultants; completed sessions emit `FAQPage` JSON-LD, so partners get SEO value.
- **Destination Wiki (§80) & Community Calendar (§86)** — embassies/universities can be authoritative sources for official dates and requirements (calendar requires an official source URL anyway).
- **Learn platform / Japa University (§36, ADR-063)** — agencies or professionals can publish paid courses with creator-economy revenue sharing.
- **Mentorship matching (§102)** — diaspora orgs and established professionals as mentors.

## 5. Trust & safety framing (do this first)

The japa space is saturated with fraudulent agents — that's exactly why the **Scam Alert Board (§81)** exists as "the highest-trust differentiator." Partnering with unvetted agents would actively damage the brand. Recommended posture:

- **Only partner with claimable/verifiable entities** — business registration, professional-body accreditation, or embassy/institutional identity.
- Keep verification visibly separate from paid placement — never let paying partners appear "endorsed."
- §55's unregistered-affiliate-link scanning (`content:scan_affiliate_links`) doubles as policing: agents who try to sneak self-promotional links into content get flagged rather than rewarded — which pushes them into the formal partner channel.

## 6. Integration-level partnerships (larger orgs)

For organizations that want programmatic integration rather than a storefront: webhooks (§12), the public API + developer portal (§47), and the MCP server (§119) let partners (e.g. a university admissions office or remittance provider) consume or push data — scholarship feeds, processing-time stats, community calendar events.

## Suggested sequencing

1. **Seed + claim**: pre-seed Service Directory listings for reputable agencies/orgs, invite them to claim (free) — builds directory density and a verified base.
2. **Verified-only monetization**: open boosts/self-serve ads only to verified orgs.
3. **Affiliate/coupon deals**: sign commission + member-discount deals with remittance, accommodation, language-test, and attestation providers via §55.
4. **Content partnerships**: AMAs and wiki authority roles for embassies, lawyers, universities.
5. **v3 bookings**: once the directory has volume, the booking pack turns listings into a transactional revenue share.

The overall pattern the spec supports: **trust first (claim → verify), then visibility (directory/ads), then transaction (affiliate/bookings)** — with every paid layer gated behind the verification layer.
