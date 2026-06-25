# ADR-033: Professional Profiles — Authorship Model

**Status:** Accepted

**Context:** Users who own agencies, consultancies, or brands need to post content attributed to that entity rather than their personal profile. The question was whether to handle this via Organization Profiles (Section 07) or a separate system, and whether authorship switching should be at the account level (change profile globally) or the content level (per-post choice).

**Decision:** A separate `professional_profiles` system, distinct from Organization Profiles (which are primarily about business listings and claim ownership). Authorship is per-post: the `contents` table has both `author_user_id` (always set — the real person, for accountability) and `author_profile_id` (optional — the professional profile to display). At content creation time, the user selects which context they're posting as from a switcher in the /create page (Section 45). Professional Profiles are followable via the universal Follow System (ADR-031). Professional Profiles have their own `professional_profile_types` (Organization, Agency, Consultancy, Brand, Practice) — admin-configurable.

**Rationale:** Keeping authorship at the per-post level rather than the account level means a user can mix personal and professional posts without switching their entire account context. Always recording `author_user_id` ensures accountability — admin can always see who the real author is even when display shows a professional profile. Separate from Organization Profiles because the use cases differ: orgs are about discovery/listings; professional profiles are about content attribution and audience-building.

**Consequences:** Content serialization must check `author_profile_id` before `author_user_id` for display name/avatar. Moderation tools must show both (the professional profile name and the underlying real user). Reports on content by professional profiles aggregate by the underlying `author_user_id` for accountability.
