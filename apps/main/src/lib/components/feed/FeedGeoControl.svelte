<script lang="ts">
	// Ref: japaconnect-engineering ADR-064 — inline "Showing: [Country] · Change"
	// geo control in the feed header. Reads content_country_preference from the
	// session user, calls PATCH /me/preferences/content-country, and invalidates
	// the feed so the new geo re-weight takes effect immediately.
	// Audit finding H1.
	// Ref: japaconnect-engineering/sections/32-activity-feed-timeline.md (Geo)
	// Ref: Rule 16 — country list fetched from /geo/countries (ADR-005), not hardcoded.
	import { invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { DropdownMenu } from 'bits-ui';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { setContentCountry } from '$lib/api/geo';
	import { Globe, ChevronDown } from 'lucide-svelte';
	import type { GeoCountry } from '$lib/types';

	let open = $state(false);
	let saving = $state(false);
	let saveError = $state<string | null>(null);

	// Country spaces fetched from the canonical /geo/countries endpoint on
	// first dropdown open. Rule 16: no hardcoded country lists.
	let countrySpaces = $state<{ code: string; label: string }[]>([]);
	let countriesLoaded = false;

	const user = $derived(page.data.user);
	const currentCountry = $derived(user?.content_country_preference ?? '');

	// Quick countries = "Auto" + the first 5 country spaces from the API.
	// The full list is available in Settings → Language & Region.
	const quickCountries = $derived([
		{ code: '', label: t('feed.geo_auto', 'Auto (GeoIP)') },
		...countrySpaces.slice(0, 5)
	]);

	async function loadCountries() {
		if (countriesLoaded) return;
		countriesLoaded = true;
		// GeoCountry rows expose the ISO country code as `geo_code`.
		const res = await api.get<GeoCountry[]>('/geo/countries');
		if (!res.error && res.data) {
			countrySpaces = res.data.map((c) => ({ code: c.geo_code, label: c.name }));
		}
	}

	async function changeCountry(code: string) {
		if (saving || code === currentCountry) {
			open = false;
			return;
		}
		saving = true;
		saveError = null;
		// Use the shared setContentCountry helper so the PATCH body field name
		// (`{ country }`) stays in sync with the backend handler and the
		// Settings page. Sending `{ content_country }` was silently clearing
		// the preference (BUG-001).
		const result = await setContentCountry(code);
		saving = false;
		if (result === null) {
			// Surface the failure — the previous version ignored the API result
			// so a failed save silently kept the old geo context.
			saveError = t('feed.geo_save_failed', 'Could not update country. Try again.');
			return;
		}
		open = false;
		// invalidateAll is required here: the feed is fetched server-side via
		// event.fetch against the backend origin, so invalidate('/feed/for-you')
		// can never match a registered dependency and the feed + the "Showing:"
		// label (page.data.user) would stay stale.
		await invalidateAll();
	}
</script>

<DropdownMenu.Root
	bind:open
	onOpenChange={(isOpen) => {
		saveError = null;
		if (isOpen) void loadCountries();
	}}
>
	<div class="relative flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
		<Globe size={13} />
		<span>{t('feed.showing', 'Showing:')}</span>
		<DropdownMenu.Trigger
			class="inline-flex items-center gap-0.5 font-medium text-[var(--color-text-subtle)] hover:text-[var(--color-text)] disabled:opacity-50"
			disabled={saving}
		>
			{currentCountry || t('feed.geo_auto', 'Auto')}
			<ChevronDown size={12} />
		</DropdownMenu.Trigger>
		{#if saveError}<span class="text-[var(--color-danger,var(--color-muted))]">{saveError}</span>{/if}
	</div>
	<DropdownMenu.Portal>
		<DropdownMenu.Content
			align="end"
			class="z-50 w-48 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-1 shadow-lg"
		>
			{#each quickCountries as c (c.code)}
				<DropdownMenu.Item
					onclick={() => changeCountry(c.code)}
					class="block w-full cursor-pointer px-3 py-1.5 text-left text-xs data-[highlighted]:bg-[var(--color-surface-subtle)] {c.code === currentCountry ? 'font-medium text-[var(--color-primary)]' : 'text-[var(--color-text-subtle)]'}"
				>
					{c.label}
				</DropdownMenu.Item>
			{/each}
			<DropdownMenu.Item
				onclick={() => { window.location.href = '/settings/language'; }}
				class="mt-1 block w-full cursor-pointer border-t border-[var(--color-border)] px-3 py-1.5 text-left text-xs text-[var(--color-primary)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
			>
				{t('feed.geo_more', 'More options…')}
			</DropdownMenu.Item>
		</DropdownMenu.Content>
	</DropdownMenu.Portal>
</DropdownMenu.Root>
