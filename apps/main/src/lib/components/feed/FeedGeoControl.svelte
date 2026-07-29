<script lang="ts">
	// Ref: japaconnect-engineering ADR-064 — inline "Showing: [Country] · Change"
	// geo control in the feed header. Reads content_country_preference from the
	// session user, calls PATCH /me/preferences/content-country, and invalidates
	// the feed so the new geo re-weight takes effect immediately.
	// Audit finding H1.
	import { invalidate } from '$app/navigation';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Globe, ChevronDown } from 'lucide-svelte';

	let changing = $state(false);
	let saving = $state(false);

	const user = $derived(page.data.user);
	const currentCountry = $derived(user?.content_country_preference ?? '');

	// A short list of common diaspora destinations. The full country list is
	// driven by the backend /spaces?level=country endpoint; this control offers
	// a quick switcher for the most common destinations plus a "More" link to
	// the full Settings → Language & Region page.
	const quickCountries = [
		{ code: '', label: 'Auto (GeoIP)' },
		{ code: 'NG', label: 'Nigeria' },
		{ code: 'US', label: 'United States' },
		{ code: 'GB', label: 'United Kingdom' },
		{ code: 'CA', label: 'Canada' },
		{ code: 'AU', label: 'Australia' }
	];

	async function changeCountry(code: string) {
		if (saving || code === currentCountry) {
			changing = false;
			return;
		}
		saving = true;
		const res = await api.patch('/me/preferences/content-country', { content_country: code });
		saving = false;
		changing = false;
		if (!res.error) {
			// Invalidate the feed so the new geo re-weight takes effect.
			await invalidate('/feed/for-you');
			await invalidate('/explore');
		}
	}
</script>

<div class="relative flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
	<Globe size={13} />
	<span>{t('feed.showing', 'Showing:')}</span>
	<button
		type="button"
		onclick={() => (changing = !changing)}
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
