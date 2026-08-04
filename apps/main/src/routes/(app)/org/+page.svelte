<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { Building2, Search, Users, BadgeCheck, ShieldX, ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { EmptyState, FollowButton, SearchInput, type SearchSuggestion } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('organizations.title', 'Organizations — JapaConnect'));
	const description = $derived(
		t('organizations.description', 'Communities, businesses, and professional groups on JapaConnect.')
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Organizations',
			description,
			url: page.url.href
		})
	);

	let query = $state(untrack(() => data.q));
	let selectedOrgType = $state(untrack(() => data.orgType));
	let selectedVerified = $state(untrack(() => data.verified));
	const searchPlaceholder = $derived(t('organizations.search_placeholder', 'Search organizations…'));

	const orgTypes = $derived([
		{ value: '', label: t('organizations.filter_all', 'All') },
		{ value: 'business', label: t('organizations.new.type_business', 'Business') },
		{ value: 'ngo', label: t('organizations.new.type_ngo', 'NGO') },
		{ value: 'government', label: t('organizations.new.type_government', 'Government') },
		{ value: 'embassy', label: t('organizations.new.type_embassy', 'Embassy') },
		{ value: 'university', label: t('organizations.new.type_university', 'University') },
		{ value: 'media', label: t('organizations.new.type_media', 'Media') },
		{ value: 'community', label: t('organizations.new.type_community', 'Community') }
	]);

	const verifiedOptions = $derived([
		{ value: '', label: t('organizations.filter_all', 'All') },
		{ value: 'true', label: t('organizations.verified', 'Verified') },
		{ value: 'false', label: t('organizations.unverified', 'Unverified') }
	]);

	function buildHref(pageNum: number): string {
		const params = new URLSearchParams();
		if (data.q) params.set('q', data.q);
		if (data.orgType) params.set('org_type', data.orgType);
		if (data.verified) params.set('verified', data.verified);
		if (pageNum > 0) params.set('page', String(pageNum));
		const qs = params.toString();
		return qs ? `/org?${qs}` : '/org';
	}

	function onFilterChange() {
		const params = new URLSearchParams();
		if (query.trim()) params.set('q', query.trim());
		if (selectedOrgType) params.set('org_type', selectedOrgType);
		if (selectedVerified) params.set('verified', selectedVerified);
		const qs = params.toString();
		goto(qs ? `/org?${qs}` : '/org');
	}

	async function orgSearchFetcher(q: string): Promise<SearchSuggestion[]> {
		const res = await api.get<{ data: { results: { type: string; id: string; title: string; excerpt?: string; url: string }[] } }>(
			`/search?q=${encodeURIComponent(q)}&type=organization&limit=8`
		);
		if (res.error || !res.data) return [];
		return (res.data.data?.results ?? []).map((r) => ({
			id: r.id,
			title: r.title,
			url: r.url,
			excerpt: r.excerpt,
			type: r.type
		}));
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header + search + filters -->
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Building2 size={20} /> {t('organizations.heading', 'Organizations')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('organizations.subtitle', 'Follow organizations to keep up with their activity.')}
		</p>

		<form class="mt-3" action="/org" method="get" role="search">
			<SearchInput
				bind:value={query}
				placeholder={searchPlaceholder}
				ariaLabel={t('organizations.heading', 'Organizations')}
				fetcher={orgSearchFetcher}
			/>
			{#if selectedOrgType}
				<input type="hidden" name="org_type" value={selectedOrgType} />
			{/if}
			{#if selectedVerified}
				<input type="hidden" name="verified" value={selectedVerified} />
			{/if}
		</form>

		<!-- Org type filter chips -->
		<div class="mt-3 flex flex-wrap gap-2">
			{#each orgTypes as ot (ot.value)}
				<button
					type="button"
					onclick={() => { selectedOrgType = ot.value; onFilterChange(); }}
					class="rounded-full border px-3 py-1 text-xs transition-colors {selectedOrgType === ot.value
						? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
						: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]/40'}"
				>
					{ot.label}
				</button>
			{/each}
		</div>

		<!-- Verified filter chips -->
		<div class="mt-2 flex flex-wrap gap-2">
			{#each verifiedOptions as vo (vo.value)}
				<button
					type="button"
					onclick={() => { selectedVerified = vo.value; onFilterChange(); }}
					class="rounded-full border px-3 py-1 text-xs transition-colors {selectedVerified === vo.value
						? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
						: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]/40'}"
				>
					{vo.label}
				</button>
			{/each}
		</div>
	</div>

	<div class="p-4">
		{#if data.forbidden}
			<div class="flex flex-col items-center gap-3 py-16 text-center">
				<ShieldX size={40} class="text-[var(--color-muted)]" />
				<p class="text-sm font-medium text-[var(--color-text)]">
					{t('organizations.forbidden', 'You do not have permission to view this directory.')}
				</p>
			</div>
		{:else if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('organizations.load_error', 'Failed to load organizations. Please try again later.')}
			</p>
		{:else if data.orgs.length === 0}
			<EmptyState icon={Building2} title={t('organizations.empty', 'No organizations yet.')} />
		{:else}
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				{data.total} {t('organizations.results', 'organizations')}
			</p>
			<div class="grid gap-3 sm:grid-cols-2">
				{#each data.orgs as org (org.id)}
					<div
						class="group flex items-start gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
					>
						<a href="/org/{org.slug}" class="flex min-w-0 flex-1 items-start gap-3">
							<span
								class="grid h-11 w-11 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
							>
								<Building2 size={20} />
							</span>
							<span class="min-w-0 flex-1">
								<span class="flex items-center gap-1.5">
									<span
										class="block truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
									>
										{org.name}
									</span>
									{#if org.verified}
										<BadgeCheck
											size={15}
											class="shrink-0 text-[var(--color-primary)]"
											aria-label={t('organizations.verified', 'Verified')}
										/>
									{/if}
								</span>
								{#if org.tagline}
									<span class="mt-1 line-clamp-2 block text-xs text-[var(--color-text-subtle)]">
										{org.tagline}
									</span>
								{/if}
								<span class="mt-2 flex items-center gap-3 text-xs text-[var(--color-muted)]">
									<span class="flex items-center gap-1">
										<Users size={12} /> {org.member_count}
									</span>
									<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 capitalize text-[var(--color-text-subtle)]">
										{org.org_type}
									</span>
								</span>
							</span>
						</a>
						{#if page.data.user}
							<FollowButton entityType="organization" entityId={org.id} entitySlug={org.slug} compact />
						{/if}
					</div>
				{/each}
			</div>

			<!-- Pagination -->
			{#if data.totalPages > 1}
				<div class="mt-6 flex items-center justify-center gap-4">
					<a
						href={buildHref(data.page - 1)}
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {data.page === 0 ? 'pointer-events-none opacity-40' : ''}"
					>
						<ChevronLeft size={16} /> {t('common.prev', 'Prev')}
					</a>
					<span class="text-xs text-[var(--color-muted)]">
						{data.page + 1} / {data.totalPages}
					</span>
					<a
						href={buildHref(data.page + 1)}
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {data.page >= data.totalPages - 1 ? 'pointer-events-none opacity-40' : ''}"
					>
						{t('common.next', 'Next')} <ChevronRight size={16} />
					</a>
				</div>
			{/if}
		{/if}
	</div>
</div>
