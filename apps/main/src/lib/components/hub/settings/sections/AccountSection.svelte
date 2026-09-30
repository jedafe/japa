<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { patchAccount, putPreferences } from '$lib/api/hub';
	import { theme as themeStore } from '$lib/stores/theme.svelte';
	import { t } from '$lib/i18n';
	import type { MeProfile, ThemeName } from '$lib/types';

	let { data }: { data: unknown } = $props();
	const profile = $derived(data as MeProfile | null);

	// Static two-value enum — theme names are a platform contract
	// (jc_theme cookie + data-theme attribute), not backend data.
	const themeOptions = [
		{ value: 'light', label: t('settings.account.theme_light', 'Light') },
		{ value: 'dark', label: t('settings.account.theme_dark', 'Dark') }
	];

	// Default to the server-resolved theme from page.data.theme (which
	// already accounts for the jc_theme cookie and the admin
	// appearance.dark_mode_default setting), then 'light'. This keeps the
	// selector in sync with the actual <html data-theme> attribute.
	const form = new DirtyForm(
		untrack(() => ({
			username: profile?.username ?? '',
			theme: (page.data.theme as ThemeName) ?? 'light'
		})),
		// PATCH /me/account only accepts username/email/display_name — the
		// previous { ui: { theme } } key was silently dropped (Rule 1 drift;
		// §50: theme persists via the jc_theme cookie, set by themeStore.set
		// on selection, and the dark_mode preference via PUT /me/preferences).
		async (v) => {
			const [acctRes, prefRes] = await Promise.all([
				patchAccount({ username: v.username }),
				putPreferences({ dark_mode: v.theme })
			]);
			const err = acctRes.error ?? prefRes.error;
			return {
				data: acctRes.data ?? prefRes.data,
				error: err ? { message: err.message, status: err.status } : null,
				status: err ? err.status : 200
			};
		}
	);

	// Live-preview the selected theme — and, because this watches
	// form.values.theme, Discard (form.reset) reverts the applied theme and
	// the jc_theme cookie too, not just the picker.
	$effect(() => {
		const th = form.values.theme;
		if (th === 'light' || th === 'dark') themeStore.set(th);
	});
</script>

<SettingsSection titleKey="settings.nav.account" descKey="settings.account.desc" {form}>
	<div class="mb-3">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.account.email')}</span>
		<p class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-muted)]">
			{profile?.email ?? '—'}
		</p>
	</div>
	<label class="mb-3 block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.account.username')}</span>
		<input
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-[var(--color-text)]"
			value={form.values.username}
			oninput={(e) => form.set({ username: e.currentTarget.value })}
		/>
	</label>
	<div class="block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.account.theme')}</span>
		<AutocompleteSelect
			options={themeOptions}
			value={form.values.theme}
			ariaLabel={t('settings.account.theme')}
			onValueChange={(v) => form.set({ theme: (v as ThemeName) || 'light' })}
		/>
	</div>

	<!-- Section 09 Product Tour: "Replay tour" link for users who want to
	     see the contextual walkthrough again. Navigates to home with ?tour=1. -->
	<div class="mt-6 border-t border-[var(--color-border)] pt-4">
		<button
			type="button"
			onclick={() => goto('/?tour=replay')}
			class="text-sm font-medium text-[var(--color-primary)] hover:underline"
		>
			{t('settings.account.replay_tour', 'Replay product tour')}
		</button>
		<p class="mt-1 text-xs text-[var(--color-muted)]">
			{t('settings.account.replay_tour_desc', 'See the walkthrough highlighting key features of the platform.')}
		</p>
	</div>
</SettingsSection>
