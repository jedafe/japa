<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { GraduationCap, Lock, Bookmark, BookmarkCheck, Search } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import type { Scholarship } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.scholarships', 'Scholarship Finder — JapaConnect'));
	const description = $derived(
		t('tools.scholarships_desc', 'Find and track scholarships by country and level.')
	);

	const scholarships = $state<Scholarship[]>(data.scholarships);
	const disabled = $state(data.disabled);
	const authed = $state(data.authed);

	// Filter state — initialized from URL.
	let q = $state(page.url.searchParams.get('q') ?? '');
	let country = $state(page.url.searchParams.get('country') ?? '');
	let level = $state(page.url.searchParams.get('level') ?? '');
	let renewable = $state(page.url.searchParams.get('renewable') ?? '');
	let loading = $state(false);
	let savedIds = $state<Set<string>>(new Set());

	function applyFilters() {
		const params = new URLSearchParams();
		if (q) params.set('q', q);
		if (country) params.set('country', country);
		if (level) params.set('level', level);
		if (renewable) params.set('renewable', renewable);
		goto(`/scholarships?${params.toString()}`, { keepFocus: true });
	}

	async function toggleSave(s: Scholarship) {
		if (!authed) return;
		if (savedIds.has(s.guide_id)) {
			await api.delete(`/scholarships/${s.guide_id}/save`);
			savedIds.delete(s.guide_id);
			savedIds = new Set(savedIds);
		} else {
			await api.post(`/scholarships/${s.guide_id}/save`);
			savedIds.add(s.guide_id);
			savedIds = new Set(savedIds);
		}
	}

	function field(s: Scholarship, key: string): string {
		return s.field_values?.[key] ?? '';
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-4xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<GraduationCap size={20} /> {t('tools.scholarships', 'Scholarship Finder')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else}
		<!-- Filter panel -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<form onsubmit={(e) => { e.preventDefault(); applyFilters(); }} class="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
				<div class="relative">
					<Search size={14} class="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
					<input
						type="text"
						placeholder={t('scholarships.search', 'Search scholarships…')}
						bind:value={q}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 pl-8 pr-3 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
					/>
				</div>
				<input
					type="text"
					placeholder={t('scholarships.country', 'Country')}
					bind:value={country}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
				/>
				<input
					type="text"
					placeholder={t('scholarships.level', 'Level')}
					bind:value={level}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
				/>
				<button
					type="submit"
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90"
				>
					{t('common.filter', 'Filter')}
				</button>
			</form>
		</div>

		<!-- Results -->
		<div class="p-4">
			{#if loading}
				<LoadingSkeleton variant="lines" />
			{:else if scholarships.length === 0}
				<EmptyState
					icon={GraduationCap}
					title={t('scholarships.empty', 'No scholarships found')}
					body={t('scholarships.empty_body', 'Try adjusting your filters.')}
				/>
			{:else}
				<div class="space-y-3">
					{#each scholarships as s (s.guide_id)}
						<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="flex items-start justify-between gap-3">
								<div class="min-w-0 flex-1">
									<h3 class="text-sm font-medium text-[var(--color-text)]">{s.title}</h3>
									{#if s.summary}
										<p class="mt-1 line-clamp-2 text-xs text-[var(--color-muted)]">{s.summary}</p>
									{/if}
									<div class="mt-2 flex flex-wrap gap-2 text-xs text-[var(--color-muted)]">
										{#if field(s, 'countries_eligible')}
											<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{field(s, 'countries_eligible')}</span>
										{/if}
										{#if field(s, 'level_of_study')}
											<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{field(s, 'level_of_study')}</span>
										{/if}
										{#if field(s, 'award_amount_usd')}
											<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">${field(s, 'award_amount_usd')}</span>
										{/if}
										{#if field(s, 'application_deadline')}
											<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">
												{t('scholarships.deadline', 'Deadline')}: {field(s, 'application_deadline')}
											</span>
										{/if}
									</div>
								</div>
								{#if authed}
									<button
										type="button"
										onclick={() => toggleSave(s)}
										class="shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] p-2 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)]"
										aria-label={savedIds.has(s.guide_id) ? t('common.saved', 'Saved') : t('common.save', 'Save')}
									>
										{#if savedIds.has(s.guide_id)}
											<BookmarkCheck size={16} class="text-[var(--color-primary)]" />
										{:else}
											<Bookmark size={16} />
										{/if}
									</button>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>
