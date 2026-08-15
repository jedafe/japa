<script lang="ts">
	/**
	 * Language & Timezone settings — Phase 18.09.
	 *
	 * Language selector populated from GET /i18n/packs (Rule 16 — no hardcoded
	 * locale list). The API returns string[] of locale codes (e.g. ["en","fr"]);
	 * display names are resolved via Intl.DisplayNames (a platform API, not app
	 * data — Rule 16 safe). Timezone selector uses the browser's
	 * Intl.supportedValuesOf API (a static list of IANA timezone names that never
	 * changes per-user or per-tenant — it is the platform's own enumeration, not
	 * app data).
	 *
	 * Language persists via PUT /me/preferences ({ language }).
	 * Timezone persists via PATCH /me/account ({ timezone }).
	 * Content-country persists via PATCH /me/preferences/content-country.
	 *
	 * Ref: japaconnect-engineering/sections/18-i18n-language-packs.md
	 *      japaconnect-engineering/sections/50-user-dashboard-settings.md
	 */
	import { Select } from 'bits-ui';
	import { untrack } from 'svelte';
	import { invalidateAll } from '$app/navigation';
	import { browser } from '$app/environment';
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import { patchAccount, putPreferences } from '$lib/api/hub';
	import { setContentCountry } from '$lib/api/geo';
	import { t, i18n } from '$lib/i18n';
	import { Globe, Clock, MapPin } from 'lucide-svelte';
	import type { GeoCountry } from '$lib/types';

	interface LanguageData {
		packs: string[];
		prefs: { language?: string | null; dark_mode?: string | null };
		profile: { timezone?: string | null; content_country_preference?: string | null };
		countries?: GeoCountry[];
	}

	let { data }: { data: unknown } = $props();
	const payload = $derived(data as LanguageData | null);
	const packs = $derived(payload?.packs ?? []);
	const prefs = $derived(payload?.prefs ?? {});
	const profile = $derived(payload?.profile ?? {});
	const countries = $derived(payload?.countries ?? []);

	// Resolve a locale code to a display name via Intl.DisplayNames (platform
	// API, not app data — Rule 16 safe). Falls back to the code itself.
	const displayNames = (() => {
		try {
			// Use the browser's default locale for display names.
			return new Intl.DisplayNames(undefined, { type: 'language' });
		} catch {
			return null;
		}
	})();

	function localeLabel(code: string): string {
		return displayNames?.of(code) ?? code;
	}

	// IANA timezone list — platform enumeration, not app data (Rule 16 safe).
	const timezones: string[] = (() => {
		try {
			return Intl.supportedValuesOf('timeZone') as string[];
		} catch {
			// Fallback for older browsers without Intl.supportedValuesOf.
			return ['UTC', 'Africa/Lagos', 'Africa/Accra', 'Europe/London', 'America/New_York'];
		}
	})();

	const browserTz = (() => {
		try {
			return Intl.DateTimeFormat().resolvedOptions().timeZone;
		} catch {
			return 'UTC';
		}
	})();

	const form = new DirtyForm(
		untrack(() => ({
			language: prefs.language ?? 'en',
			timezone: profile.timezone ?? browserTz
		})),
		// Language saves via PUT /me/preferences; timezone saves via PATCH /me/account.
		// Both fire in parallel — they are independent fields on different endpoints.
		async (v) => {
			const [langRes, tzRes] = await Promise.all([
				putPreferences({ language: v.language }),
				patchAccount({ timezone: v.timezone })
			]);
			// Surface the first error if either call failed.
			const err = langRes.error ?? tzRes.error;
			// If the language preference persisted and differs from the active UI
			// locale, set the jc_locale cookie (mirroring the locale-switch
			// routes) and invalidate all loaders. The i18n store derives its
			// locale from page.data.locale (request-scoped during SSR,
			// reactive on the client), so after invalidateAll() refreshes the page
			// data the whole UI re-renders in the chosen language automatically.
			// Ref: japa-bugs-hunters.md §1 / §21 — without this the preference
			// saved silently and the UI never switched language.
			if (!langRes.error && browser && v.language && v.language !== i18n.locale) {
				document.cookie = `jc_locale=${v.language}; path=/; max-age=${60 * 60 * 24 * 365}; samesite=lax`;
				// Fire-and-forget: re-run load functions so data.locale refreshes.
				void invalidateAll();
			}
			return {
				data: langRes.data ?? tzRes.data,
				error: err ? { message: err.message, status: err.status } : null,
				status: err ? err.status : 200
			};
		}
	);

	// Content-country preference (ADR-064) — separate from the DirtyForm
	// because it uses a different endpoint (PATCH /me/preferences/content-country)
	// and has its own cache-invalidation semantics.
	let contentCountry = $state(untrack(() => profile.content_country_preference ?? ''));
	let savingCountry = $state(false);
	let countryError = $state<string | null>(null);

	async function saveContentCountry(value: string) {
		const prev = contentCountry;
		contentCountry = value;
		savingCountry = true;
		countryError = null;
		// setContentCountry returns null on failure (api.patch yields null
		// data on error). Roll back the optimistic update so the trigger
		// reflects the server's actual state instead of a phantom selection.
		// Ref: japa-bugs-hunters.md §1.1 — optimistic update without rollback.
		const res = await setContentCountry(value);
		savingCountry = false;
		if (!res) {
			contentCountry = prev;
			countryError = t('settings.language.content_country_error', 'Could not save content country preference.');
		}
	}
</script>

<SettingsSection titleKey="settings.nav.language" descKey="settings.language.desc" {form}>
	<label class="mb-4 block">
		<span class="mb-1.5 flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
			<Globe size={14} /> {t('settings.language.language', 'Language')}
		</span>
		<Select.Root
			type="single"
			value={form.values.language}
			onValueChange={(v) => v && form.set({ language: v })}
		>
			<Select.Trigger
				class="inline-flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				aria-label={t('settings.language.language', 'Language')}
			>
				{localeLabel(form.values.language)}
			</Select.Trigger>
			<Select.Portal>
				<Select.Content class="z-50 max-h-60 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg">
					<Select.Viewport>
						{#each packs as code (code)}
							<Select.Item
								value={code}
								label={localeLabel(code)}
								class="flex cursor-pointer items-center justify-between rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
							>
								<span>{localeLabel(code)}</span>
								<span class="text-xs text-[var(--color-muted)]">{code}</span>
							</Select.Item>
						{/each}
					</Select.Viewport>
				</Select.Content>
			</Select.Portal>
		</Select.Root>
	</label>

	<label class="block">
		<span class="mb-1.5 flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
			<Clock size={14} /> {t('settings.language.timezone', 'Timezone')}
		</span>
		<Select.Root
			type="single"
			value={form.values.timezone}
			onValueChange={(v) => v && form.set({ timezone: v })}
		>
			<Select.Trigger
				class="inline-flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				aria-label={t('settings.language.timezone', 'Timezone')}
			>
				{form.values.timezone}
			</Select.Trigger>
			<Select.Portal>
				<Select.Content class="z-50 max-h-60 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg">
					<Select.Viewport>
						{#each timezones as tz (tz)}
							<Select.Item
								value={tz}
								label={tz}
								class="cursor-pointer rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
							>
								{tz}
							</Select.Item>
						{/each}
					</Select.Viewport>
				</Select.Content>
			</Select.Portal>
		</Select.Root>
		<p class="mt-1.5 text-xs text-[var(--color-muted)]">
			{t('settings.language.timezone_hint', 'Times will be displayed in your selected timezone.')}
		</p>
	</label>

	{#if countries.length > 0}
		<label class="mt-4 block">
			<span class="mb-1.5 flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
				<MapPin size={14} /> {t('settings.language.content_country', 'Content country')}
			</span>
			<Select.Root
				type="single"
				value={contentCountry}
				onValueChange={(v) => v !== undefined && saveContentCountry(v ?? '')}
			>
				<Select.Trigger
					class="inline-flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
					aria-label={t('settings.language.content_country', 'Content country')}
				>
					{contentCountry
						? countries.find((c) => c.geo_code === contentCountry)?.name ?? contentCountry
						: t('settings.language.content_country_auto', 'Auto-detect from my location')}
				</Select.Trigger>
				<Select.Portal>
					<Select.Content class="z-50 max-h-60 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg">
						<Select.Viewport>
							<Select.Item
								value=""
								label={t('settings.language.content_country_auto', 'Auto-detect from my location')}
								class="flex cursor-pointer items-center justify-between rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
							>
								{t('settings.language.content_country_auto', 'Auto-detect from my location')}
							</Select.Item>
							{#each countries as c (c.id)}
								<Select.Item
									value={c.geo_code}
									label={c.name}
									class="flex cursor-pointer items-center justify-between rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
								>
									<span>{c.name}</span>
									<span class="text-xs text-[var(--color-muted)]">{c.geo_code}</span>
								</Select.Item>
							{/each}
						</Select.Viewport>
					</Select.Content>
				</Select.Portal>
			</Select.Root>
			<p class="mt-1.5 text-xs text-[var(--color-muted)]">
				{t('settings.language.content_country_hint', 'Personalizes your feed and search results. Leave on auto to use your detected location.')}
			</p>
			{#if countryError}<p class="mt-1 text-xs text-[var(--color-danger,#dc2626)]">{countryError}</p>{/if}
		</label>
	{/if}
</SettingsSection>
