<script lang="ts">
	// Ref: japaconnect-engineering ADR-064 — inline "Showing: [Country] · Change"
	// geo control in the feed header. Reads content_country_preference from the
	// session user, calls PATCH /me/preferences/content-country, and invalidates
	// the feed so the new geo re-weight takes effect immediately.
	// Audit finding H1.
	// Ref: japaconnect-engineering/sections/32-activity-feed-timeline.md (Geo)
	// Ref: Rule 16 — country list fetched from /spaces?level=country, not hardcoded.
	import { invalidate } from '$app/navigation';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { setContentCountry } from '$lib/api/geo';
	import { Globe, ChevronDown } from 'lucide-svelte';

	let changing = $state(false);
	let saving = $state(false);

	// Country spaces fetched from the backend on first dropdown open.
	// Rule 16: no hardcoded country lists — the backend /spaces?level=country
	// endpoint is the single source of truth.
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
		// Space rows expose the ISO country code as `geo_code` (Space type,
		// apps/main/src/lib/types/index.ts). Not `country_code`.
		const res = await api.get<{ items: { geo_code: string | null; name: string }[] }>(
			'/spaces?level=country&page_size=5'
		);
		if (!res.error && res.data?.items) {
			countrySpaces = res.data.items
				.filter((s) => s.geo_code)
				.map((s) => ({ code: s.geo_code as string, label: s.name }));
		}
	}

	async function changeCountry(code: string) {
		if (saving || code === currentCountry) {
			changing = false;
			return;
		}
		saving = true;
		// Use the shared setContentCountry helper so the PATCH body field name
		// (`{ country }`) stays in sync with the backend handler and the
		// Settings page. Sending `{ content_country }` was silently clearing
		// the preference (BUG-001).
		await setContentCountry(code);
		saving = false;
		changing = false;
		// Invalidate the feed so the new geo re-weight takes effect.
		await invalidate('/feed/for-you');
		await invalidate('/explore');
	}

	async function toggleDropdown() {
		changing = !changing;
		if (changing) {
			await loadCountries();
		}
	}
</script>

<div class="relative flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
	<Globe size={13} />
	<span>{t('feed.showing', 'Showing:')}</span>
	<button
		type="button"
		onclick={toggleDropdown}
		class="inline-flex items-center gap-0.5 font-medium text-[var(--color-text-subtle)] hover:text-[var(--color-text)]"
		aria-expanded={changing}
	>
		{currentCountry || t('feed.geo_auto', 'Auto')}
		<ChevronDown size={12} />
	</button>
	{#if changing}
		<div class="absolute right-0 top-full z-20 mt-1 w-48 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-1 shadow-lg">
			{#each quickCountries as c (c.code)}
				<button
					type="button"
					onclick={() => changeCountry(c.code)}
					class="block w-full px-3 py-1.5 text-left text-xs hover:bg-[var(--color-surface-subtle)] {c.code === currentCountry ? 'font-medium text-[var(--color-primary)]' : 'text-[var(--color-text-subtle)]'}"
				>
					{c.label}
				</button>
			{/each}
			<a
				href="/dashboard/settings?section=language"
				class="mt-1 block border-t border-[var(--color-border)] px-3 py-1.5 text-xs text-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]"
			>
				{t('feed.geo_more', 'More options…')}
			</a>
		</div>
	{/if}
</div>
