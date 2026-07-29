<script lang="ts">
	/**
	 * Language & Timezone settings — Phase 18.09.
	 *
	 * Language selector populated from GET /i18n/packs (Rule 16 — no hardcoded
	 * locale list). Timezone selector uses the browser's Intl.supportedValuesOf
	 * API (a static list of IANA timezone names that never changes per-user or
	 * per-tenant — it is the platform's own enumeration, not app data). Both
	 * persist via PATCH /me/account with `language_preference` and `timezone`.
	 *
	 * Ref: japaconnect-engineering/sections/18-i18n-language-packs.md
	 *      japaconnect-engineering/sections/50-user-dashboard-settings.md
	 */
	import { Select } from 'bits-ui';
	import { untrack } from 'svelte';
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import { patchAccount } from '$lib/api/hub';
	import { setContentCountry } from '$lib/api/geo';
	import { t } from '$lib/i18n';
	import { Globe, Clock, MapPin } from 'lucide-svelte';
	import type { GeoCountry } from '$lib/types';

	interface LanguagePack {
		code: string;
		name: string;
		native_name?: string | null;
	}

	interface LanguageData {
		packs: LanguagePack[];
		current: { language_preference?: string | null; timezone?: string | null; content_country_preference?: string | null };
		countries?: GeoCountry[];
	}

	let { data }: { data: unknown } = $props();
	const payload = $derived(data as LanguageData | null);
	const packs = $derived(payload?.packs ?? []);
	const current = $derived(payload?.current ?? {});
	const countries = $derived(payload?.countries ?? []);

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
			language_preference: current.language_preference ?? 'en',
			timezone: current.timezone ?? browserTz
		})),
		(v) => patchAccount({ language_preference: v.language_preference, timezone: v.timezone })
	);

	// Content-country preference (ADR-064) — separate from the DirtyForm
	// because it uses a different endpoint (PATCH /me/preferences/content-country)
	// and has its own cache-invalidation semantics.
	let contentCountry = $state(untrack(() => current.content_country_preference ?? ''));
	let savingCountry = $state(false);

	async function saveContentCountry(value: string) {
		contentCountry = value;
		savingCountry = true;
		await setContentCountry(value);
		savingCountry = false;
	}
</script>

<SettingsSection titleKey="settings.nav.language" descKey="settings.language.desc" {form}>
	<label class="mb-4 block">
		<span class="mb-1.5 flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
			<Globe size={14} /> {t('settings.language.language', 'Language')}
		</span>
		<Select.Root
			type="single"
			value={form.values.language_preference}
			onValueChange={(v) => v && form.set({ language_preference: v })}
		>
			<Select.Trigger
				class="inline-flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				aria-label={t('settings.language.language', 'Language')}
			>
				{packs.find((p) => p.code === form.values.language_preference)?.native_name
					?? packs.find((p) => p.code === form.values.language_preference)?.name
					?? form.values.language_preference}
			</Select.Trigger>
			<Select.Portal>
				<Select.Content class="z-50 max-h-60 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg">
					<Select.Viewport>
						{#each packs as pack (pack.code)}
							<Select.Item
								value={pack.code}
								label={pack.native_name ?? pack.name}
								class="flex cursor-pointer items-center justify-between rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
							>
								<span>{pack.native_name ?? pack.name}</span>
								<span class="text-xs text-[var(--color-muted)]">{pack.code}</span>
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
		</label>
	{/if}
</SettingsSection>
