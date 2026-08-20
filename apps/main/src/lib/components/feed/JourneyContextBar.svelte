<script lang="ts">
	// Ref: 2026-08-21-journey-aware-recommendations-design.md §6.3
	// Rule 16: country names are resolved from /geo/countries.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { getFeedContext } from '$lib/api/journey';
	import type { GeoCountry, VisitorContextSummary } from '$lib/types';
	import { Pencil } from 'lucide-svelte';

	interface Props {
		onEdit?: () => void;
	}

	let { onEdit }: Props = $props();

	let loaded = $state(false);
	let countryList = $state<GeoCountry[]>([]);
	let ctx = $state<VisitorContextSummary | null>(null);

	$effect(() => {
		if (loaded) return;
		loaded = true;
		void (async () => {
			const [countriesRes, ctxRes] = await Promise.all([
				api.get<GeoCountry[]>('/geo/countries'),
				getFeedContext()
			]);
			if (countriesRes.data) {
				countryList = countriesRes.data;
			}
			if (ctxRes.data) {
				ctx = ctxRes.data;
			}
		})();
	});

	function countryName(code: string): string {
		if (!code) return '';
		const c = countryList.find((x) => x.geo_code === code);
		return c?.name ?? code;
	}

	function hasContext(c: VisitorContextSummary | null): boolean {
		if (!c) return false;
		return !!(c.origin_country || c.destination_country);
	}

	function summary(c: VisitorContextSummary | null): string {
		if (!c) return '';
		const parts: string[] = [];
		if (c.origin_country && c.destination_country) {
			parts.push(`${countryName(c.origin_country)} → ${countryName(c.destination_country)}`);
		} else if (c.destination_country) {
			parts.push(countryName(c.destination_country));
		} else if (c.origin_country) {
			parts.push(countryName(c.origin_country));
		}
		if (c.purpose) parts.push(c.purpose);
		if (c.visa_route) parts.push(c.visa_route);
		if (c.journey_stage) parts.push(c.journey_stage);
		if (c.travel_date) parts.push(c.travel_date);
		return parts.join(' · ');
	}
</script>

{#if hasContext(ctx)}
	<div class="border-b border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-2">
		<div class="flex items-center justify-between gap-3">
			<div class="min-w-0">
				<p class="text-sm text-[var(--color-text-subtle)]">
					<span class="font-medium text-[var(--color-text)]">{t('journey.bar.label', 'Your journey:')}</span>
					<span class="truncate">{summary(ctx)}</span>
				</p>
			</div>
			<button
				type="button"
				class="shrink-0 rounded p-1 text-[var(--color-muted)] hover:bg-[var(--color-surface)] hover:text-[var(--color-text)]"
				onclick={() => onEdit?.()}
				aria-label={t('journey.bar.edit', 'Edit journey')}
			>
				<Pencil size={16} />
			</button>
		</div>
	</div>
{/if}
