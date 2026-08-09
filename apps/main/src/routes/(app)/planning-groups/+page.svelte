<script lang="ts">
	// Ref: japaconnect-engineering/sections/82-japa-planning-groups.md
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Users, Plus, MapPin, Plane, Calendar, Search, Lock, Unlock } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('planning_groups.title', 'Planning Groups — JapaConnect'));
	const description = $derived(
		t(
			'planning_groups.description',
			'Join or create groups to plan your japa journey with others heading to the same destination.'
		)
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Planning Groups',
			description,
			url: page.url.href
		})
	);

	let destFilter = $state(data.filters.dest);
	let visaFilter = $state(data.filters.visa);

	const filterHref = $derived(() => {
		const params = new URLSearchParams();
		if (destFilter.trim()) params.set('dest', destFilter.trim());
		if (visaFilter.trim()) params.set('visa', visaFilter.trim());
		const qs = params.toString();
		return qs ? `/planning-groups?${qs}` : '/planning-groups';
	});
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
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<div>
			<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
				<Users size={20} /> {t('planning_groups.heading', 'Planning Groups')}
			</h1>
			<p class="mt-0.5 text-sm text-[var(--color-text-subtle)]">
				{t('planning_groups.subtitle', 'Plan your japa journey together.')}
			</p>
		</div>
		{#if data.authed}
			<a
				href="/planning-groups/new"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<Plus size={14} /> {t('planning_groups.create', 'Create Group')}
			</a>
		{:else}
			<a
				href="/login?redirect=/planning-groups"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<Plus size={14} /> {t('planning_groups.create', 'Create Group')}
			</a>
		{/if}
	</div>

	<!-- Filter bar -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<form method="GET" action="/planning-groups" class="flex flex-wrap items-end gap-3">
			<div class="flex flex-col gap-1">
				<label for="filter-dest" class="text-xs font-medium text-[var(--color-text-subtle)]">
					{t('planning_groups.filter_destination', 'Destination')}
				</label>
				<div class="relative">
					<MapPin
						size={14}
						class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-muted)]"
					/>
					<input
						id="filter-dest"
						type="text"
						name="dest"
						value={destFilter}
						oninput={(e) => (destFilter = e.currentTarget.value)}
						placeholder={t('planning_groups.filter_destination_placeholder', 'e.g. Canada')}
						class="w-44 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-1.5 pl-8 pr-3 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
					/>
				</div>
			</div>
			<div class="flex flex-col gap-1">
				<label for="filter-visa" class="text-xs font-medium text-[var(--color-text-subtle)]">
					{t('planning_groups.filter_visa', 'Visa category')}
				</label>
				<div class="relative">
					<Plane
						size={14}
						class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-muted)]"
					/>
					<input
						id="filter-visa"
						type="text"
						name="visa"
						value={visaFilter}
						oninput={(e) => (visaFilter = e.currentTarget.value)}
						placeholder={t('planning_groups.filter_visa_placeholder', 'e.g. Student')}
						class="w-44 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-1.5 pl-8 pr-3 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
					/>
				</div>
			</div>
			<button
				type="submit"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				<Search size={14} /> {t('planning_groups.filter_apply', 'Filter')}
			</button>
			{#if data.filters.dest || data.filters.visa}
				<a
					href="/planning-groups"
					class="text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
				>
					{t('common.clear', 'Clear')}
				</a>
			{/if}
		</form>
	</div>

	<div class="p-4">
		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('planning_groups.load_error', 'Failed to load planning groups. Please try again later.')}
			</p>
		{:else if data.groups.length === 0}
			<EmptyState
				icon={Users}
				title={t('planning_groups.empty', 'No planning groups yet.')}
				body={t(
					'planning_groups.empty_body',
					'Be the first to create a group for your destination and visa category.'
				)}
				action={data.authed ? { label: t('planning_groups.create', 'Create Group'), href: '/planning-groups/new' } : undefined}
			/>
		{:else}
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each data.groups as group (group.id)}
					<a
						href="/planning-groups/{group.id}"
						class="group flex flex-col gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
					>
						<div class="flex items-start justify-between gap-2">
							<h2
								class="truncate text-sm font-semibold text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
							>
								{group.name}
							</h2>
							<span
								class="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]"
								title={group.is_open_join
									? t('planning_groups.open_join', 'Open to join')
									: t('planning_groups.closed_join', 'Invite only')}
							>
								{#if group.is_open_join}
									<Unlock size={12} />
								{:else}
									<Lock size={12} />
								{/if}
							</span>
						</div>

						{#if group.description}
							<p class="line-clamp-2 text-xs text-[var(--color-text-subtle)]">{group.description}</p>
						{/if}

						<div class="mt-auto flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--color-muted)]">
							{#if group.visa_category}
								<span class="inline-flex items-center gap-1">
									<Plane size={12} /> {group.visa_category}
								</span>
							{/if}
							{#if group.target_departure_window}
								<span class="inline-flex items-center gap-1">
									<Calendar size={12} /> {group.target_departure_window}
								</span>
							{/if}
						</div>

						<div class="flex items-center justify-between border-t border-[var(--color-border)] pt-3 text-xs text-[var(--color-muted)]">
							<span class="inline-flex items-center gap-1">
								<Users size={12} />
								{group.max_members ?? 0}
							</span>
							{#if group.visa_category}
								<span>{group.visa_category}</span>
							{/if}
						</div>
					</a>
				{/each}
			</div>
		{/if}
	</div>
</div>
