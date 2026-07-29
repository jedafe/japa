<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { Building2, Search, Users, BadgeCheck } from 'lucide-svelte';
	import { EmptyState, FollowButton } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('organizations.title', 'Organizations — JapaConnect'));
	const description = $derived(
		t(
			'organizations.description',
			'Communities, businesses, and professional groups on JapaConnect.'
		)
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
	const searchPlaceholder = $derived(
		t('organizations.search_placeholder', 'Search organizations…')
	);
	let filtered = $derived(
		query.trim()
			? data.orgs.filter((o) =>
					o.name.toLowerCase().includes(query.trim().toLowerCase())
				)
			: data.orgs
	);
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

<div class="mx-auto w-full max-w-4xl">
	<!-- Header + search -->
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Building2 size={20} /> {t('organizations.heading', 'Organizations')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('organizations.subtitle', 'Follow organizations to keep up with their activity.')}
		</p>

		<form class="mt-3" action="/organizations" method="get" role="search">
			<div
				class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 focus-within:border-[var(--color-primary)]"
			>
				<Search size={16} class="text-[var(--color-muted)]" />
				<input
					type="search"
					name="q"
					placeholder={searchPlaceholder}
					aria-label={t('organizations.heading', 'Organizations')}
					bind:value={query}
					class="w-full bg-transparent text-sm text-[var(--color-text)] outline-none placeholder:text-[var(--color-muted)]"
				/>
			</div>
		</form>
	</div>

	<div class="p-4">
		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('organizations.load_error', 'Failed to load organizations. Please try again later.')}
			</p>
		{:else if data.orgs.length === 0}
			<EmptyState icon={Building2} title={t('organizations.empty', 'No organizations yet.')} />
		{:else if filtered.length === 0}
			<EmptyState
				icon={Search}
				title={t('organizations.no_match', 'No organizations match your search.')}
			/>
		{:else}
			<div class="grid gap-3 sm:grid-cols-2">
				{#each filtered as org (org.id)}
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
									<span
										class="mt-1 line-clamp-2 block text-xs text-[var(--color-text-subtle)]"
									>
										{org.tagline}
									</span>
								{/if}
								<span
									class="mt-2 flex items-center gap-3 text-xs text-[var(--color-muted)]"
								>
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
		{/if}
	</div>
</div>
