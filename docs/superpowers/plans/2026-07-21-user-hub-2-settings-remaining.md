# User Hub — Plan 2: Settings (Security & the Rest)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use `- [ ]`. Load **jc-main-develop** and **svelte** for component tasks. **Depends on Plan 1** (`2026-07-21-user-hub-1-infrastructure-settings-core.md`) being complete — reuses its registry, rail, `useDirtyForm`, `optimisticAction`, `SettingsSection`.

**Goal:** Complete the `/settings` family — security (password/TOTP/sessions), connected accounts, notifications, API keys, affiliate links, wallet, subscription, and the danger zone — by wiring each into Plan 1's machinery. `devices` stays `backendReady:false` (no component).

**Architecture:** Each section is a component registered in `settingsSections.ts` (flipping its `component: null` to the built component) with a server loader added to `settingsLoaders.server.ts`. Form sections reuse `SettingsSection` + `useDirtyForm`; discrete actions use `optimisticAction`; the four special flows (TOTP, API-key reveal, delete-account, change-password) are dedicated sub-components.

**Tech Stack:** As Plan 1.

**Spec:** `docs/superpowers/specs/2026-07-21-user-hub-design.md` §6 + the special flows.

## Global Constraints

(Identical to Plan 1 — Svelte 5 strict Runes, Bits UI dot-notation, Tailwind tokens, i18n en+fr, Rule 16, zero `any`, no AI attribution, all-green baseline, per-task commits on `main` adding only that task's files.)

- **Secret-shown-once (spec §6):** API-key create/rotate and TOTP backup codes are returned ONCE. The reveal UI must capture the response value and show it before any dismissal; a failed create shows nothing.
- **Terminal action:** `DELETE /me` ends the session → confirm dialog → await → hard redirect to a logged-out route. Not `optimisticAction`.

## File Structure

```
src/lib/api/hub.ts ...................... (MODIFY) add all remaining mutation wrappers
src/lib/components/hub/settings/
  settingsSections.ts ................... (MODIFY) import + wire the 8 built components
  settingsLoaders.server.ts ............. (MODIFY) add loaders for each section
  security.ts ........................... (CREATE) pure: validatePassword
  security.test.ts ...................... (CREATE)
  danger.ts ............................. (CREATE) pure: matchesConfirmation
  danger.test.ts ........................ (CREATE)
  sections/
    NotificationsSection.svelte ......... (CREATE)
    ConnectedSection.svelte ............. (CREATE)
    SecuritySection.svelte ............. (CREATE)
    ApiKeysSection.svelte .............. (CREATE)
    AffiliateSection.svelte ........... (CREATE)
    WalletSection.svelte .............. (CREATE)
    SubscriptionSection.svelte ........ (CREATE)
    DangerSection.svelte .............. (CREATE)
  flows/
    ChangePasswordForm.svelte ......... (CREATE)
    TotpEnrollFlow.svelte ............. (CREATE)
    ApiKeyRevealDialog.svelte ......... (CREATE)
    DeleteAccountDialog.svelte ........ (CREATE)
src/routes/(app)/settings/logged-out/+page.svelte  (CREATE) post-deletion landing
src/lib/i18n/en.json, fr.json ........... (MODIFY)
```

---

## Phase 0 — Pure logic + wrappers

### Task 1: `validatePassword` (TDD)

**Files:** Create `src/lib/components/hub/settings/security.ts`, `security.test.ts`.

**Interfaces:** Produces `validatePassword(current: string, next: string, confirm: string): { ok: boolean; reason: string | null }` (reason is an i18n key).

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect } from 'vitest';
import { validatePassword } from './security';

describe('validatePassword', () => {
	it('requires the current password', () => {
		expect(validatePassword('', 'Abcd1234!', 'Abcd1234!').reason).toBe('settings.security.err_current');
	});
	it('enforces a minimum strength (>=8 with letter+number)', () => {
		expect(validatePassword('x', 'short', 'short').reason).toBe('settings.security.err_weak');
		expect(validatePassword('x', 'allletters', 'allletters').reason).toBe('settings.security.err_weak');
	});
	it('requires confirm to match', () => {
		expect(validatePassword('x', 'Abcd1234', 'Abcd9999').reason).toBe('settings.security.err_mismatch');
	});
	it('passes a strong matching pair', () => {
		expect(validatePassword('x', 'Abcd1234', 'Abcd1234').ok).toBe(true);
	});
});
```

- [ ] **Step 2:** Run `cd apps/main && bun run test settings/security` → FAIL.
- [ ] **Step 3: Implement**

```ts
export function validatePassword(
	current: string,
	next: string,
	confirm: string
): { ok: boolean; reason: string | null } {
	if (current.length === 0) return { ok: false, reason: 'settings.security.err_current' };
	const strong = next.length >= 8 && /[a-zA-Z]/.test(next) && /[0-9]/.test(next);
	if (!strong) return { ok: false, reason: 'settings.security.err_weak' };
	if (next !== confirm) return { ok: false, reason: 'settings.security.err_mismatch' };
	return { ok: true, reason: null };
}
```

- [ ] **Step 4:** Run → PASS. **Step 5:** Commit `feat(main): password validation helper`.

### Task 2: `matchesConfirmation` (TDD)

**Files:** Create `src/lib/components/hub/settings/danger.ts`, `danger.test.ts`.

**Interfaces:** Produces `matchesConfirmation(typed: string, expected: string): boolean` (exact, trimmed, case-sensitive).

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect } from 'vitest';
import { matchesConfirmation } from './danger';

describe('matchesConfirmation', () => {
	it('matches the exact username (trimmed)', () => {
		expect(matchesConfirmation('  jane ', 'jane')).toBe(true);
	});
	it('is case-sensitive and rejects near-misses', () => {
		expect(matchesConfirmation('Jane', 'jane')).toBe(false);
		expect(matchesConfirmation('jan', 'jane')).toBe(false);
		expect(matchesConfirmation('', 'jane')).toBe(false);
	});
});
```

- [ ] **Step 2:** Run `cd apps/main && bun run test settings/danger` → FAIL.
- [ ] **Step 3: Implement**

```ts
export function matchesConfirmation(typed: string, expected: string): boolean {
	return typed.trim() === expected && expected.length > 0;
}
```

- [ ] **Step 4:** Run → PASS. **Step 5:** Commit `feat(main): delete-account confirmation matcher`.

### Task 3: hub client wrappers + server loaders (all remaining sections)

**Files:** Modify `src/lib/api/hub.ts`, `settingsLoaders.server.ts`.

- [ ] **Step 1: Add mutation wrappers to `hub.ts`** (verify each path/body against the handler when its section task runs):

```ts
// security
export const changePassword = (current: string, next: string) =>
	api.post('/me/change-password', { current_password: current, new_password: next });
export const totpStatus = () => api.get<{ enabled: boolean }>('/me/totp/status');
export const totpEnable = () => api.post<{ secret: string; otpauth_url: string }>('/me/totp/enable');
export const totpConfirm = (code: string) => api.post<{ backup_codes: string[] }>('/me/totp/confirm', { code });
export const totpDisable = () => api.delete('/me/totp');
export const revokeSession = (id: string) => api.delete(`/me/sessions/${id}`);
export const revokeOtherSessions = () => api.delete('/me/sessions');
// connected
export const unlinkAccount = (provider: string) => api.delete(`/me/connected-accounts/${provider}`);
// notifications
export const patchNotifications = (v: Record<string, unknown>) => api.patch('/me/notification-preferences', v);
// api keys
export const createApiKey = (v: { name: string; scopes: string[] }) => api.post<{ id: string; key: string }>('/me/api-keys', v);
export const rotateApiKey = (id: string) => api.post<{ key: string }>(`/me/api-keys/${id}/rotate`);
export const updateApiKey = (id: string, v: { name?: string; scopes?: string[] }) => api.patch(`/me/api-keys/${id}`, v);
export const revokeApiKey = (id: string) => api.delete(`/me/api-keys/${id}`);
// affiliate
export const createAffiliate = (v: { target_url: string; label?: string }) => api.post('/me/affiliate-links', v);
export const deleteAffiliate = (id: string) => api.delete(`/me/affiliate-links/${id}`);
// wallet / subscription
export const convertWallet = (points: number) => api.post('/me/wallet/convert', { points });
export const cancelSubscription = () => api.post('/me/subscription/cancel');
export const reactivateSubscription = () => api.post('/me/subscription/reactivate');
// danger
export const exportData = () => api.post('/me/data-export');
export const deactivateAccount = () => api.post('/me/deactivate');
export const deleteAccount = () => api.delete('/me');
```

- [ ] **Step 2: Add server loaders** to `settingsLoaders.server.ts`:

```ts
security: async (ctx) => ({
	totp: await apiGet(ctx, '/me/totp/status').catch(() => ({ enabled: false })),
	sessions: (await apiList(ctx, '/me/sessions')).items
}),
connected: async (ctx) => (await apiList(ctx, '/me/connected-accounts')).items,
notifications: (ctx) => apiGet(ctx, '/me/notification-preferences'),
'api-keys': async (ctx) => (await apiList(ctx, '/me/api-keys')).items,
affiliate: async (ctx) => (await apiList(ctx, '/me/affiliate-links')).items,
wallet: async (ctx) => ({
	wallet: await apiGet(ctx, '/me/wallet'),
	ledger: (await apiList(ctx, '/me/wallet/ledger')).items
}),
subscription: (ctx) => apiGet(ctx, '/me/subscription'),
danger: async () => null
```

- [ ] **Step 3:** Run `cd apps/main && bun run check` → PASS. **Step 4:** Commit `feat(main): settings client wrappers + loaders (remaining sections)`.

---

## Phase 1 — Simple form/action sections

### Task 4: Notifications section (nested-map dirty form)

**Files:** Create `sections/NotificationsSection.svelte`; wire in `settingsSections.ts`.

**Behavior:** loads a nested preference map; each toggle updates `form.set({ ...form.values, [key]: value })`; save → `patchNotifications(form.values)`. Proves `useDirtyForm`'s nested-map dirty detection (already unit-tested in Plan 1). Wrap in `SettingsSection`. Toggle rows use accessible checkboxes/switches.

- [ ] Step 1: Implement (mirror Plan 1 `ProfileSection` structure; fields = the preference map keys). Step 2: flip `settingsSections.ts` entry `notifications` → `component: NotificationsSection`. Step 3: `bun run check` PASS. Step 4: Commit `feat(main): notifications section`.

### Task 5: Connected accounts (list + optimistic unlink)

**Files:** Create `sections/ConnectedSection.svelte`; wire in registry.

**Behavior:** render `data` (linked providers); each has an **Unlink** button using `optimisticAction({ apply: remove from local list, rollback: re-add, call: () => unlinkAccount(provider) })`. Linking is out of scope (spec Risk) — show a note if no start endpoint. Uses `DashboardPanel`-less plain layout (no dirty form; it's action-only). Empty state via `EmptyState`.

- [ ] Step 1: Implement. Step 2: wire registry. Step 3: `bun run check` PASS. Step 4: Commit `feat(main): connected accounts section`.

### Task 6: Affiliate links (mini-CRUD)

**Files:** Create `sections/AffiliateSection.svelte`; wire in registry.

**Behavior:** list `data`; create form (`target_url` + optional `label` → `createAffiliate`, prepend to local list); delete per row via `optimisticAction(deleteAffiliate)`. `EmptyState` when none.

- [ ] Step 1: Implement. Step 2: wire registry. Step 3: `bun run check` PASS. Step 4: Commit `feat(main): affiliate links section`.

### Task 7: Wallet (read + convert)

**Files:** Create `sections/WalletSection.svelte`; wire in registry.

**Behavior:** show balances from `data.wallet` (points/credits) as `MetricCard`s; ledger table from `data.ledger`; a convert form (points → credits via `convertWallet`) with success/refresh. Read-mostly.

- [ ] Step 1: Implement. Step 2: wire registry. Step 3: `bun run check` PASS. Step 4: Commit `feat(main): wallet section`.

### Task 8: Subscription (read + cancel/reactivate)

**Files:** Create `sections/SubscriptionSection.svelte`; wire in registry.

**Behavior:** show plan/status/renewal from `data`; **Cancel** (confirm `Dialog` → `cancelSubscription`) or **Reactivate** (`reactivateSubscription`) depending on state, via `optimisticAction`.

- [ ] Step 1: Implement. Step 2: wire registry. Step 3: `bun run check` PASS. Step 4: Commit `feat(main): subscription section`.

---

## Phase 2 — Special flows

### Task 9: Change password + Security section shell + sessions

**Files:** Create `flows/ChangePasswordForm.svelte`, `sections/SecuritySection.svelte`; wire in registry.

**Behavior:**
- `ChangePasswordForm`: three fields (current/new/confirm); on submit, `validatePassword` (Task 1) → if `!ok` show `reason`; else `changePassword` → success clears the form, error surfaces. Action form (not dirty-tracked).
- `SecuritySection`: composes `ChangePasswordForm`, the sessions list (from `data.sessions` — each row shows device/last-active + **Revoke** via `optimisticAction(revokeSession)`, mark current, plus **Revoke all others** `revokeOtherSessions`), and mounts `TotpEnrollFlow` (Task 10).

- [ ] Step 1: Implement `ChangePasswordForm`. Step 2: Implement `SecuritySection` (password + sessions; TOTP slot filled in Task 10). Step 3: wire registry `security` → component. Step 4: `bun run check` PASS. Step 5: Commit `feat(main): security section (password + sessions)`.

### Task 10: TOTP enrollment flow

**Files:** Create `flows/TotpEnrollFlow.svelte`; mount in `SecuritySection`.

**Behavior:** local state machine seeded from `data.totp.enabled`:
```
disabled ─Enable→ totpEnable() ─(secret+otpauth shown)→ enrolling
enrolling ─enter code · totpConfirm(code)→ enabled (backup_codes shown ONCE in a Dialog)
enabled ─Disable (confirm)· totpDisable()→ disabled
```
Local `$state` for `step`, `secret`, `otpauthUrl`, `backupCodes`. Render the secret + `otpauth://` URI (spec Risk: manual entry now; QR optional if a generator is vendored). Backup codes shown in a Bits UI `Dialog` with copy; dialog can't be dismissed until acknowledged.

- [ ] Step 1: Implement the flow component. Step 2: `bun run check` PASS. Step 3: Commit `feat(main): TOTP 2FA enrollment flow`.

### Task 11: API keys section + reveal dialog (secret-once)

**Files:** Create `sections/ApiKeysSection.svelte`, `flows/ApiKeyRevealDialog.svelte`; wire in registry.

**Behavior:**
- List `data` (name, prefix, scopes, created). Row actions: **Rotate** (`rotateApiKey` → reveal new key), **Edit** (`updateApiKey` name/scopes), **Revoke** (confirm → `revokeApiKey`).
- **Create**: name + scopes → `createApiKey` → open `ApiKeyRevealDialog` with the returned `key` (shown once, copy button, "you won't see this again"); on close, list refreshes to show prefix only.
- `ApiKeyRevealDialog` captures the key value in local state from the create/rotate response BEFORE it can be dismissed.
- IP allowlist add/remove per key (optional within this task; `POST/DELETE …/ip-allowlist`).

- [ ] Step 1: Implement `ApiKeyRevealDialog`. Step 2: Implement `ApiKeysSection`. Step 3: wire registry `api-keys` → component. Step 4: `bun run check` PASS. Step 5: Commit `feat(main): API keys section + secret-once reveal`.

### Task 12: Danger zone + delete-account dialog

**Files:** Create `sections/DangerSection.svelte`, `flows/DeleteAccountDialog.svelte`, `routes/(app)/settings/logged-out/+page.svelte`; wire in registry.

**Behavior:**
- `DangerSection`: **Export data** (`exportData` → "we'll email you" state), **Deactivate** (confirm `Dialog` → `deactivateAccount`, reversible), **Delete account** (opens `DeleteAccountDialog`).
- `DeleteAccountDialog`: shows the username; a text input; the confirm button enables only when `matchesConfirmation(typed, username)` (Task 2). On confirm → `deleteAccount()` → on success `goto('/settings/logged-out')` (or the app's logout route); on failure stay + error.
- `logged-out/+page.svelte`: minimal "your account was deleted" page.

- [ ] Step 1: Implement `DeleteAccountDialog` (uses `matchesConfirmation`). Step 2: Implement `DangerSection` + logged-out page. Step 3: wire registry `danger` → component. Step 4: `bun run check` PASS. Step 5: Commit `feat(main): danger zone (export/deactivate/delete)`.

---

## Phase 3 — i18n + verification

### Task 13: i18n keys + full gate + live round-trip

**Files:** Modify `en.json`, `fr.json`.

- [ ] Step 1: `cd apps/main && grep -rohE "settings\.[a-z_.]+" src/lib/components/hub/settings src/routes/\(app\)/settings | sort -u` → add every new key to en+fr.
- [ ] Step 2: `cd apps/main && bun run test && bun run check` → all PASS.
- [ ] Step 3: Live round-trip (backend :8080): change password (weak → blocked; strong → success); enroll TOTP → confirm → backup codes shown once → disable; create an API key → key shown once → reload shows prefix only; revoke a session; cancel/reactivate subscription; delete-account button stays disabled until the exact username is typed (do NOT actually delete in a shared env — verify gating only).
- [ ] Step 4: Final commit (if fixes) `fix(main): settings remaining round-trip corrections`.

---

## Self-Review (plan author)

- **Spec coverage:** security/password/TOTP/sessions (T1,T9,T10) · connected (T5) · notifications (T4) · api-keys + secret-once (T11) · affiliate (T6) · wallet (T7) · subscription (T8) · danger + typed-delete (T2,T12) · devices stays blocked (untouched, Plan 1's `backendReady:false`).
- **Type consistency:** all wrappers (T3) consumed by their sections; `validatePassword`/`matchesConfirmation` (T1,T2) consumed by T9/T12; each section flips exactly its own `settingsSections.ts` entry.
- **Secret-once + terminal-action invariants** (spec §6, §8) implemented in T11/T12 respectively; delete uses `goto`, not `optimisticAction`.
