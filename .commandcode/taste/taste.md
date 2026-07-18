# Taste (Continuously Learned by [CommandCode][cmd])

[cmd]: https://commandcode.ai/

# git
- Stage and commit ALL modified files (including submodule pointer changes), not just task-specific ones, when running `git add`. Confidence: 0.75

# audit
- When fixing audit findings, verify the audit file for remaining ⚠️ and ❌ items before declaring all findings resolved. Confidence: 0.80
- Mark fixes as "**Already Fixed**" in the audit file, not in code comments. Confidence: 0.70

# ui
- LayerChart is part of the admin panel tech stack and should be included in skill/architecture documentation for apps/admin/. Confidence: 0.65

# testing
- Use Playwright for end-to-end testing in this project. Confidence: 0.65

# data
- Use Wikidata SPARQL for university ranking data instead of paid APIs. Confidence: 0.50

# audit
- When fixing audit findings, don't mark issues as P5/DEFERRED — actually implement the fixes. Confidence: 0.65

# i18n
- All user-facing strings must use `t()` with proper i18n keys — no hardcoded strings in code. Manage translations via the admin panel (not by editing JSON files directly). Confidence: 0.95
- New locales can be created by copying all keys from `en` via the admin panel's "Add Language" feature (batch-upserts to backend). Confidence: 0.80
- Missing i18n keys in catalog can be auto-detected by scanning for t('key') calls not in en.json — add key:value to en.json and sync fr.json. Confidence: 0.70
- Auto-wrap hardcoded markup text nodes safely by guarding against Svelte block expressions ({#if, {#each), JS operators (&&, ||), and script blocks. Confidence: 0.75

# workflow
- When encountering orphaned/disconnected code (unwired tasks, services, etc.), wire them up instead of deleting — don't remove anything that will be useful. Confidence: 0.75

# permissions
- Super admin groups should be granted the `*` wildcard permission rather than maintaining explicit permission key mappings. Confidence: 0.65

