# ADR-046: i18n — French as Second Language (Session-4 Override)

**Status:** Accepted

**Context:** Session-3 proposed English-only for V1 to reduce translation maintenance burden. Session-4 explicitly overrode this: "Add a second language 'French', keep the default url structure for english and /fr/... for French. Add the possibility for admin to Create New Language Pack and assign UserGroups to manage Language Strings in admin panel."

**Decision:** V1 ships with English (root, default) and French (`/fr/...`). Further languages deferred. Admin Language Pack system: `languages` + `translation_keys` + `translations` + `language_pack_managers` tables. Workflow: admin creates pack → AI auto-populates via `platform_agent.translate_string` tool → assigned UserGroup reviews in Admin Panel → admin publishes at discretion. Missing keys always fall back to English (never blank). hreflang tags emitted for `en` and `fr` on all public pages. Runtime-resolved (not compile-time bundled — see ADR-013).

**Rationale:** French is the most widely spoken language in West Africa after English and is an official language in several countries in the JapaConnect diaspora's primary origin and destination regions (Francophone West Africa, Canada, Belgium, Switzerland, France). The admin-managed Language Pack system means future languages can be added without code changes. The AI auto-populate step makes initial translation tractable at solo-founder scale.

**Consequences:** All public-facing strings must be internationalised from the first commit — retrofitting i18n is extremely expensive. The translation key namespace must be maintained in sync with the codebase (a CI lint step can check for untranslated string literals in SvelteKit templates). The French translation initial pass will require HITL review — the AI will produce grammatically correct but potentially culturally awkward translations for diaspora-specific terminology.
