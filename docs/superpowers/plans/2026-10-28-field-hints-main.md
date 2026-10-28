# Field Hints — Pass 1: apps/main Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every form field in `apps/main` gets contextual helper text and a placeholder, via i18n keys (`en.json` + inline `t()` fallback).

**Architecture:** Surgical per-file application of a canonical hint pattern — no shared component migration. Helper text is a muted `<span>` under each field linked via `aria-describedby`. `PasswordField` gets an optional `hint` prop since it is the shared password control.

**Tech Stack:** SvelteKit 2, Svelte 5 Runes, Tailwind v4 (CSS vars like `var(--color-muted)`), app i18n via `t(key, fallback?)` from `$lib/i18n`.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-10-28-field-hints-design.md` — all rules below come from it.
- **i18n:** `t()` accepts a fallback: `t('area.fields.name_hint', 'English copy')`. Always pass the English copy as the fallback AND add the key to `src/lib/i18n/en.json`. Do NOT touch `fr.json`/`jce_*.json` (fr falls back to en).
- **Key naming:** `<area>.fields.<name>_hint` / `<area>.fields.<name>_placeholder`, where `<area>` matches the file's existing key group (e.g. `auth.fields.*`, `settings.profile.*`, `tools.remittance.*`). Reuse the group's existing prefix — do not invent new top-level groups.
- **Helper copy:** one short sentence (≤ ~90 chars) explaining *why*, *format*, or a *constraint* — never a restatement of the label. Good: "Used for sign-in and receipts — we never share it." Bad: "Enter your email."
- **Placeholder copy:** a concrete example value, not a label repeat. Good: `you@example.com`. Bad: `Email`.
- **Existing placeholders:** keep them (do not churn); optionally move hardcoded placeholder strings into `t()` keys when adding the hint anyway.
- **Exclusions** — NO helper text; placeholder only if missing and clearly beneficial: search boxes / command palettes, chat/message/comment composers (DM input, forum reply box, comment editor, JapaBot prompt), DataTable/filter inputs, hidden inputs (honeypot), submit buttons, read-only display inputs.
- **Selects/checkboxes:** get helper text when a choice needs context; never placeholder.
- **a11y:** hint `<span>` gets `id="{field-id}-hint"`; the control gets `aria-describedby="{field-id}-hint"`. When the field also has an error element, use `aria-describedby={error ? '{id}-hint {id}-error' : '{id}-hint'}`. If a control has no `id`, add one (kebab-case, unique in file).
- **Style:** match each file's existing markup — most fields use `text-xs`/`text-sm` with `text-[var(--color-muted)]`; hint renders BELOW the control (and below any error message goes after the hint).
- **No new components** except the `hint` prop on `PasswordField` (Task 1). Custom components like `AutocompleteSelect` get the hint `<span>` at the callsite.
- **Verify:** `cd apps/main && bun run check`. Baseline recorded 2026-10-28: **0 errors, 14 warnings** (all unused-CSS warnings in `routes/(app)/roadmap/releases/*`). The pass is clean only if your changes add ZERO new errors.
- **en.json:** keys appear roughly sorted/grouped — insert new keys adjacent to their group.
- **Enumeration must include custom field components:** many fields wrap `AutocompleteSelect`, `DestinationSelector`, `PasswordField`, or a `<select>`-like component inside `<label>` with NO raw `<input>` tag — grep each file for `<label` as well as `<input|<textarea|<select` and inspect label children.

## Canonical pattern (every task applies this)

Standard labeled field (label wraps control — the dominant pattern in `main`):

```svelte
<label class="block">
	<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('area.fields.name')}</span>
	<input
		id="unique-field-id"
		placeholder={t('area.fields.name_placeholder', 'example value')}
		aria-describedby="unique-field-id-hint"
		<!-- existing attrs unchanged -->
	/>
	<span id="unique-field-id-hint" class="mt-1 block text-xs text-[var(--color-muted)]">
		{t('area.fields.name_hint', 'Why/format hint copy')}
	</span>
</label>
```

Field with error display (auth pattern — `for`/`id` association):

```svelte
<label for="login-email" class="sr-only">{t('auth.fields.email')}</label>
<input
	id="login-email"
	placeholder={t('auth.fields.email_placeholder', 'you@example.com')}
	aria-describedby={errors.email ? 'login-email-hint login-email-error' : 'login-email-hint'}
/>
<span id="login-email-hint" class="text-xs text-[var(--color-muted)]">
	{t('auth.fields.email_hint', "Used for sign-in and receipts — we never share it.")}
</span>
{#if errors.email}<p id="login-email-error" class="text-sm text-red-500">{t(errors.email)}</p>{/if}
```

Custom component callsite (e.g. `AutocompleteSelect`, `TagInput`):

```svelte
<label class="block">
	<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('area.fields.name')}</span>
	<AutocompleteSelect ... ariaDescribedby="area-name-hint" />
	<span id="area-name-hint" class="mt-1 block text-xs text-[var(--color-muted)]">
		{t('area.fields.name_hint', 'hint copy')}
	</span>
</label>
```

(If the component lacks an `aria-describedby` pass-through, render the hint span anyway — visible copy matters most; note the gap, don't redesign the component.)

---

### [DONE] Task 1: Shared `PasswordField` hint prop + auth surfaces

**Files:**
- Modify: `src/lib/components/auth/PasswordField.svelte` — add optional `hint?: string` prop
- Modify: `src/lib/components/auth/LoginForm.svelte`
- Modify: `src/lib/components/auth/SignupForm.svelte`
- Modify: `src/routes/(auth)/forgot-password/+page.svelte`
- Modify: `src/routes/(auth)/reset-password/+page.svelte`
- Modify: `src/routes/(auth)/verify-totp/+page.svelte`
- Modify: `src/routes/unsubscribe/+page.svelte`
- Keys: `src/lib/i18n/en.json` (under `auth.fields.*`, `auth.totp.*`, `unsubscribe.*`)
- Exclude: `src/lib/components/auth/HoneypotField.svelte` (hidden input)

**Interfaces:**
- Produces: `PasswordField` gains `hint?: string` — later tasks reuse it: `<PasswordField hint={t('area.fields.password_hint', 'copy')} />`. When `hint` is set the component renders `<p id="{id}-hint" class="text-xs text-[var(--color-muted)]">{hint}</p>` below the strength meter and sets `aria-describedby` to include `{id}-hint` (plus `{id}-error` when `error` is truthy).

- [ ] **Step 1: Baseline check** — run `cd apps/main && bun run check 2>&1 | tail -5`; record the error count.
- [ ] **Step 2: Add `hint` prop to `PasswordField`** — add `hint = ''` to the `$props()` destructure and `hint?: string` to the type; change `aria-describedby` on the input to `{[hint && `${id}-hint`, error && `${id}-error`].filter(Boolean).join(' ') || undefined}`; render `{#if hint}<p id="{id}-hint" class="text-xs text-[var(--color-muted)]">{hint}</p>{/if}` after the strength meter block, before the error block.
- [ ] **Step 3: Apply hints to the 6 auth files** — follow the error-pattern snippet above. Email gets `auth.fields.email_hint` = "Used for sign-in and receipts — we never share it." Password on login gets `auth.fields.password_hint` = "Your JapaConnect account password." Signup password: pass `hint={t('auth.fields.new_password_hint', 'At least 8 characters — mix letters, numbers and symbols.')}`; username gets a format hint; TOTP code: `auth.totp.code_hint` = "6-digit code from your authenticator app."; unsubscribe email reuses `auth.fields.email_hint`.
- [ ] **Step 4: Add all new keys to `en.json`.**
- [ ] **Step 5: Re-run `bun run check`** — zero new errors vs baseline.
- [ ] **Step 6: Commit** — `git add` the touched files + `en.json`; commit `feat(main): add field hints to auth forms`.

### [DONE] Task 2: Settings hub — profile, account & preference sections

**Files:** `src/lib/components/hub/settings/sections/`: `ProfileSection.svelte`, `AccountSection.svelte`, `LocationSection.svelte`, `LanguageSection.svelte`, `NotificationsSection.svelte`, `PrivacySection.svelte`, `AIPrivacySection.svelte`. Keys: `src/lib/i18n/en.json` under `settings.*`.

- [ ] **Step 1: Enumerate** — read all 7 files; list every `input`/`textarea`/`select`/`AutocompleteSelect`. Skip any search/filter inputs.
- [ ] **Step 2: Apply pattern** — each field gets `placeholder` (inputs only, where missing), `id` (if missing), `aria-describedby`, and the hint `<span>` below the control. Copy examples: `settings.profile.bio_hint` = "Shown on your public profile — keep it under a few lines." `settings.profile.website_hint` = "Your personal site or portfolio, shown on your profile." `settings.notifications.*_hint` = explains what each toggle controls. `AutocompleteSelect` fields (profession, education) get the callsite hint pattern.
- [ ] **Step 3: Add keys to `en.json`** under `settings.profile.*`, `settings.account.*`, `settings.notifications.*`, `settings.privacy.*`, `settings.ai_privacy.*`, `settings.location.*`, `settings.language.*` — match whatever group the file already uses.
- [ ] **Step 4:** `bun run check` — zero new errors.
- [ ] **Step 5: Commit** — `feat(main): add field hints to settings profile/preference sections`.

### [DONE] Task 3: Settings hub — monetization, keys & flow dialogs

**Files:** `src/lib/components/hub/settings/sections/`: `NewsletterSection.svelte`, `ApiKeysSection.svelte`, `AIKeysSection.svelte`, `AffiliateSection.svelte`, `SubscriptionSection.svelte`, `MentorSection.svelte`, `DangerSection.svelte`; `flows/`: `ChangePasswordForm.svelte`, `TotpEnrollFlow.svelte`, `DeleteAccountDialog.svelte`, `ApiKeyRevealDialog.svelte`; `wallet/`: `WalletPurchaseTab.svelte`, `WalletConvertTab.svelte`, `WalletWithdrawTab.svelte`, `WalletDetailsTab.svelte`. Keys: `en.json` under each section's existing group.

- [ ] **Step 1: Enumerate** all fields in the 15 files.
- [ ] **Step 2: Apply pattern.** Copy examples: API key name = "A label so you remember what this key is for." Wallet withdraw address/amount = format + minimum/fee note where the backend exposes one (use generic copy otherwise, e.g. "Amount in wallet credits to withdraw."). `ChangePasswordForm` reuses `PasswordField` with its `hint` prop. `DeleteAccountDialog` confirm field: "Type DELETE to confirm." `TotpEnrollFlow` code field reuses the TOTP hint style from Task 1.
- [ ] **Step 3: Keys → `en.json`.**
- [ ] **Step 4:** `bun run check` — zero new errors.
- [ ] **Step 5: Commit** — `feat(main): add field hints to settings keys/wallet/flows`.

### [DONE] Task 4: Onboarding, profile & user dialogs

**Files:** `src/routes/onboarding/+page.svelte`, `src/lib/components/feed/JourneyOnboardingModal.svelte`, `src/lib/components/profile/JourneysPanel.svelte`, `MessageUserDialog.svelte`, `ReportDialog.svelte`, `src/lib/components/referral/ReferralWidget.svelte`, `src/lib/components/appeals/AppealButton.svelte`, `src/lib/components/landing/GuestHomePage.svelte`, `src/lib/components/marketplace/MarketplaceHero.svelte`, `src/routes/(app)/claim-ownership/+page.svelte`, `src/routes/(app)/u/[username]/claim/+page.svelte`, `src/routes/(app)/verification-request/+page.svelte`, `src/routes/(app)/dashboard/bookmarks/+page.svelte`. Keys: each area's existing group in `en.json`.

- [ ] **Step 1: Enumerate** — classify each input; message-body textareas in `MessageUserDialog`/`ReportDialog`/`AppealButton` ARE form fields (they submit a report/message), give them hints like "Include links or details that help the review." Search inputs in `GuestHomePage`/`MarketplaceHero`/bookmarks are search boxes → placeholder only.
- [ ] **Step 2: Apply pattern.** Onboarding fields are the highest-value hints (first-run UX): e.g. destination-country select = "Where you're planning to move — powers your roadmap." japa-stage/step fields get format hints.
- [ ] **Step 3: Keys → `en.json`.**
- [ ] **Step 4:** `bun run check` — zero new errors.
- [ ] **Step 5: Commit** — `feat(main): add field hints to onboarding and user dialogs`.

### [DONE] Task 5: Create panel, spaces & editors

**Files:** `src/lib/components/create/panel/`: `CreateJourneyPanel.svelte`, `CreateSeoPanel.svelte`, `CreateSettingsPanel.svelte`, `SeriesCreationModal.svelte`; `src/lib/components/create/editors/`: `ComparisonBuilder.svelte`, `ListBuilder.svelte`; `src/lib/components/create/`: `TagInput.svelte`, `VideoField.svelte`, `CustomFieldGroup.svelte`; `src/lib/components/spaces/`: `SpaceCreationModal.svelte`, `SpaceSettings.svelte`, `SpaceCombobox.svelte`; `src/lib/components/create/CreateShell.svelte`; `src/lib/components/shell/SidebarWidgetRenderer.svelte`; `src/routes/(app)/dashboard/spaces/+page.svelte`. Keys: `en.json`.

- [ ] **Step 1: Enumerate** fields.
- [ ] **Step 2: Apply pattern.** SEO panel: title/slug/description get length/format hints ("55–60 chars shows fully in search results."). `TagInput`: "Press Enter to add a tag." `VideoField`: accepted URL formats. `SpaceCombobox`: "Search spaces or type to create." Settings panel toggles get one-line effect notes.
- [ ] **Step 3: Keys → `en.json`.**
- [ ] **Step 4:** `bun run check` — zero new errors.
- [ ] **Step 5: Commit** — `feat(main): add field hints to create panel and spaces`.

### [DONE] Task 6: Community feature pages

**Files:** `src/routes/(app)/achievements/{+page,new/+page,[slug]/edit/+page}.svelte`, `ama/{new,[id]/edit}/+page.svelte`, `calendar/{+page,new,[id]/edit}/+page.svelte`, `forum/{[category]/new,[slug]}/+page.svelte`, `japa-stage/{+page,me/+page}.svelte`, `mutual-aid/{+page,new,[id]/edit}/+page.svelte`, `planning-groups/{+page,new,[id],[id]/edit}/+page.svelte`, `roadmap/submit/+page.svelte`, `scam-alerts/{+page,new,[id]/edit}/+page.svelte`, `stories/+page.svelte`, `feedback/+page.svelte`, `tags/{+page,[slug]}/+page.svelte`, `spaces/{+page,[slug]}/+page.svelte`, `wiki/[space_slug]/{create,[page_slug]}/+page.svelte`, `src/lib/components/wiki/SuggestEdit.svelte`, `WikiRollupPanel.svelte`. Keys: `en.json`.

- [ ] **Step 1: Enumerate.** Forum reply box + comment-like inputs = composer → placeholder only. `new`/`edit` page fields = full pattern.
- [ ] **Step 2: Apply pattern.** Examples: scam-alert URL field = "Link to the listing or profile if you have one." Event date/time = timezone note. Wiki page title = "Use a clear, searchable title — it becomes the URL slug."
- [ ] **Step 3: Keys → `en.json`.**
- [ ] **Step 4:** `bun run check` — zero new errors.
- [ ] **Step 5: Commit** — `feat(main): add field hints to community feature forms`.

### [DONE] Task 7: Marketplace, org, jobs & services

**Files:** `src/lib/components/deals/DealForm.svelte`, `src/lib/components/marketplace/{MentorshipRequestDialog,MessageDialog,BookingDialog}.svelte`, `src/lib/components/messages/BookingDetailPane.svelte`, `src/lib/components/jobs/JobGeoFields.svelte`, `src/lib/components/services/{ServiceContentEditor,ServiceTierEditor}.svelte`, `src/routes/(app)/deal/[provider]/+page.svelte`, `deals/new/+page.svelte`, `(marketplace)/services/+page.svelte`, `jobs/{+page,create,[slug],[slug]/edit}/+page.svelte`, `services/{new,[slug]/edit}/+page.svelte`, `org/{+page,new,[slug]/coupons,[slug]/jobs/[jobId],[slug]/service-bookings}/+page.svelte`, `me/{applications,import}/+page.svelte`, `dashboard/organizations/{+page,[id],[id]/ads}/+page.svelte`, `dashboard/tickets/{new,[id]}/+page.svelte`. Keys: `en.json`.

- [ ] **Step 1: Enumerate** — booking/mentorship message textareas are form fields (get hints); any search/filter inputs get placeholder only.
- [ ] **Step 2: Apply pattern.** Examples: job salary fields = currency/range hint; coupon code = "Uppercase letters and numbers, e.g. JAPA20." Service tier price/delivery = unit hints. Ticket subject/body = support-context hints.
- [ ] **Step 3: Keys → `en.json`.**
- [ ] **Step 4:** `bun run check` — zero new errors.
- [ ] **Step 5: Commit** — `feat(main): add field hints to marketplace/org/jobs forms`.

### [DONE] Task 8: Tools pages

**Files:** all under `src/routes/(app)/tools/`: `cost-of-living`, `credential-checker`, `crs-calculator`, `eca-guide`, `language-test`, `pof-calculator`, `processing-tracker/{+page,my-applications}`, `remittance`, `sop-generator`, `travel`, `visa-routes`, `arrival-checklist`, `attestation-guide`, `community-locator`, `currency-converter`, `cv-formatter`, `embassy-alerter`, `readiness`, `visa-checklist`, `housing-guide`, `tax-calculator` — each `+page.svelte`, plus the shared field component `src/lib/components/tools/DestinationSelector.svelte`. Keys: `en.json` under `tools.<tool>.*`.

- [ ] **Step 1: Enumerate** all tool form fields.
- [ ] **Step 2: Apply pattern.** Tool inputs are question-like fields — hints explain what the value drives: e.g. CRS calculator age = "Your age on the day you'd submit an Express Entry profile." POF amount = "Funds in your account, in CAD." remittance amount = source-currency note.
- [ ] **Step 3: Keys → `en.json`.**
- [ ] **Step 4:** `bun run check` — zero new errors.
- [ ] **Step 5: Commit** — `feat(main): add field hints to tools forms`.

### [DONE] Task 9: AI, studio, sources & remaining surfaces

**Files:** `src/lib/ai/{AddAiKeyDialog,EditAiKeyDialog,AiBodyField,AiDraftPanel,AiModelVoicePicker,ChatTemplatePanel}.svelte`, `src/lib/components/compare/AiQuickPanel.svelte`, `src/lib/components/studio/{ImportPanel,ContentManagerView,PromptRightPanel,PromptBuilder}.svelte`, `src/lib/components/source/{ProposeSourceForm,SourceEditDialog}.svelte`, `src/lib/components/reviews/ReviewForm.svelte`, `src/lib/components/ui/LiteralSuggest.svelte`, `src/lib/components/hub/dashboard/views/{AnalyticsView,RelationshipsView}.svelte`, routes: `sources/+page`, `scholarships/{+page,[guide_id],saved}/+page`, `universities/{+page,[slug]}/+page`, `visa-timelines/{+page,my}/+page`, `compare/+page`, `comparisons/+page`, `dashboard/media/+page`, `series/+page`, `deals/+page`, `messages/requests/+page`, `studio/{videos,moderation}/+page`, `search/+page`. Keys: `en.json`.

- [ ] **Step 1: Enumerate** — `search/+page` and filter inputs are exclusions (placeholder only); `AiBodyField`/`AiDraftPanel`/`PromptRightPanel` prompt textareas are AI composers → placeholder only, no helper. `AddAiKeyDialog`/`EditAiKeyDialog` fields get full pattern.
- [ ] **Step 2: Apply pattern** to the genuine form fields (source propose/edit, review form, AI key name+key fields, import forms, media metadata fields). Example: AI key field = "Stored encrypted — used only for your AI features."
- [ ] **Step 3: Keys → `en.json`.**
- [ ] **Step 4:** `bun run check` — zero new errors.
- [ ] **Step 5: Commit** — `feat(main): add field hints to ai/studio/source forms`.

### Task 10: Verification sweep

**Files:** none new — review only.

- [ ] **Step 1: Sweep for missed fields** — run `grep -rln --include="*.svelte" -E "<input|<textarea|<select" src` in `apps/main`, diff against files touched; for each untouched file confirm it only contains excluded inputs (search/chat/filter/hidden) — spot-read any doubtful file.
- [ ] **Step 2: Sweep for unhinted labeled fields** — `grep -rn --include="*.svelte" "<label" src | grep -v "aria-describedby\|sr-only"` and manually inspect hits; some labels legitimately wrap exclusions — classify each.
- [ ] **Step 3: Full verification** — `bun run check` (zero new errors vs original baseline) and `bun run build` succeeds.
- [ ] **Step 4: Manual spot-check** — run `bun run dev`, open login page + settings profile + one tool page; visually confirm hint text renders under fields, placeholders show, layout not broken.
- [ ] **Step 5: Commit** any sweep fixes — `feat(main): field hints verification sweep`.

---

## Progress log (execution state)

- **PASSES 4-5 DONE**: academy (20 spans, instructor/track/apply/checkout/appeal forms, commit feat(academy)) and docs (9 spans, page editor + API key/playground forms, commit feat(docs)). Both svelte-check clean.
- **PASS 6 (mobile) DONE**: JcTextField hint prop→helperText in packages/japaconnect_core + 6 auth fields (login/register/totp). Commit: feat(mobile).
- **REMAINING**: Pass 7 studio-desktop (~91 fields — shared `prompt/` components TextField/SelectField/NumberField/CheckboxGroup get a `hint` prop centrally), Pass 8 extension (~1 field inline).: Pass 6 mobile (Flutter — auth screens use shared `JcTextField` widget: add `hint` param → `helperText`; fields: login email/password, register fields, totp code — ~12; composers/reply/search excluded), Pass 7 studio-desktop (~91 fields — add `hint` prop to shared `prompt/` field components TextField/SelectField/NumberField/CheckboxGroup), Pass 8 extension (~1 field, inline hint).
- **PASS 3 (control) DONE**: 60 helper spans across 17 files + 49 keys in apps/control/src/lib/locales/en.json. svelte-check: 0 errors. Commit: feat(control): add field hints across ops forms.
- **PASS 2 (admin) DONE**: 536 FieldDef.help props + 94 formSnippet helper spans across 88+16 route files; ~500 keys in apps/admin/src/lib/locales/en.json. svelte-check: 0 errors. Commit: feat(admin): add field hints across entity forms. Remaining passes: control, academy, docs, mobile, studio-desktop, extension.

- Tasks 1–9 DONE & committed. Task 10 sweep done: corrupted scripted insertions reverted+reapplied (label-anchored), duplicate hints removed where pages had pre-existing guidance, svelte-check clean (0 errors / 14 baseline warnings). Remaining intentionally-unhinted files: composers (AiBodyField uses own placeholder prop — fields still show placeholder), search/filter/sort inputs, dynamic admin-authored renderers (CustomFieldGroup, claim-ownership, verification-request, readiness), hidden honeypots, readonly share fields, literal-suggest combobox, Acknowledge checkbox w/ inline label, ApiKeyRevealDialog checkbox. NOT verified: visual spot-check via bun run dev.
- Earlier partial note: Tasks 1–5 DONE + Task 6 PARTIAL + Task 7 PARTIAL (DealForm, deals/new, MentorshipRequestDialog, BookingDialog, jobs/create, services/new, org/new, coupons, tickets/new committed — 28 hints). STILL TODO Task 7: MessageDialog(textarea—reuse services.message_hint), BookingDetailPane, JobGeoFields(country/state/city), ServiceTierEditor+ServiceContentEditor(inline tier cells—aria-labels already), deal/[provider], marketplace/services, jobs list/[slug]/edit, services/[slug]/edit, org+page/jobs[slug]/service-bookings, me/applications+import, dashboard/organizations×3, tickets/[id]. (all `new` forms committed: achievements, ama, calendar, forum, mutual-aid, planning-groups, roadmap submit, scam-alerts — 38 hints). STILL TODO for Task 6: matching `[id]/edit`+`[slug]/edit` twins (same field sets — apply same hint keys), stories/feedback(dynamic renderer—skip)/tags/spaces/wiki (SuggestEdit, WikiRollupPanel), japa-stage/me, list pages w/ real fields (ama[?], achievements edit). & committed (auth, settings profile/prefs, settings keys/wallet/flows, onboarding/dialogs). svelte-check clean: 0 errors / 14 baseline warnings.
- **Key inserter:** `/tmp/add_keys.py` — run from `apps/main`, JSON on stdin, inserts sorted into `src/lib/i18n/en.json`. Recreate if gone (json.load → sorted insert → dump indent=2 ensure_ascii=False + trailing newline).
- **Commit surgically:** `git add` exact files only — tree has ~60 unrelated dirty files; a broad `git add apps/main/src` already caused one bad commit (fixed via `git reset --soft`).
- **Intentionally skipped:** dynamic field renderers (CustomFieldGroup, claim-ownership, verification-request — admin-authored labels), readonly inputs (ReferralWidget share URL), hidden honeypots, search/composer inputs (MarketplaceHero, bookmarks search), MentorSection (already had help text), NotificationsSection/AIPrivacySection/LocationSection (already hint-complete).
- Task 5 remaining: CreateJourneyPanel inputs, CreateSeoPanel (title/desc → length hints), CreateSettingsPanel (slug/canonical/publish_at), SeriesCreationModal (3), ComparisonBuilder, ListBuilder, TagInput, VideoField, CreateShell, SpaceCreationModal, SpaceSettings, SpaceCombobox, SidebarWidgetRenderer (email widget), dashboard/spaces (search only — skip).
