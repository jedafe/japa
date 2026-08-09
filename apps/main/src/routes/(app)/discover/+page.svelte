<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Compass, Users, Building2, Hash, GraduationCap } from 'lucide-svelte';
	import { EmptyState, UrlTabs, type UrlTab } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tab = $derived(data.tab);

	// Tab bar values — switching calls goto() so SvelteKit re-runs the load
	// function for the new tab's per-tab data.
	const tabs = $derived<UrlTab[]>([
		{ value: 'spaces', label: t('discover.tab.spaces', 'spaces') },
		{ value: 'people', label: t('discover.tab.people', 'people') },
		{ value: 'organizations', label: t('discover.tab.organizations', 'organizations') },
		{ value: 'tags', label: t('discover.tab.tags', 'tags') },
		{ value: 'universities', label: t('discover.tab.universities', 'universities') }
	]);

	const title = $derived(t('discover.title', 'Discover — JapaConnect'));
	const description = $derived(
		t('discover.description', 'Find spaces, people, organizations, tags, and universities in the Nigerian diaspora community.')
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="text-lg font-semibold text-[var(--color-text)]">{t('discover.heading', 'Discover')}</h1>
	</div>

	<!-- Tab bar -->
	<nav
		class="discover-tabs sticky top-0 z-10"
		aria-label={t('discover.tabs_label', 'Discover tabs')}
	>
		<UrlTabs {tabs} value={data.tab} onValueChange={(v) => goto(`?tab=${v}`)} />
	</nav>

	<div class="p-3">
		{#if tab === 'spaces'}
			{#if data.spaces.length === 0}
				<EmptyState icon={Compass} title={t('discover.spaces_empty', 'No space suggestions yet')} />
			{:else}
				<ul class="space-y-2">
					{#each data.spaces as space (space.id)}
						<li>
							<a href="/spaces/{space.slug}" class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:bg-[var(--color-surface-subtle)]">
								<span class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">{space.name.charAt(0).toUpperCase()}</span>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">{space.name}</span>
									<span class="block text-xs text-[var(--color-muted)]">{t('meta.members', '{n} members').replace('{n}', String(space.member_count))}</span>
								</span>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if tab === 'people'}
			{#if data.people.length === 0}
				<EmptyState icon={Users} title={t('discover.people_empty', 'No people suggestions yet')} />
			{:else}
				<ul class="space-y-2">
					{#each data.people as person (person.id)}
						<li>
							<a href="/u/{person.username}" class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:bg-[var(--color-surface-subtle)]">
								{#if person.avatar_url}
									<img src={person.avatar_url} alt="" class="h-10 w-10 rounded-full object-cover" />
								{:else}
									<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">{person.display_name.charAt(0).toUpperCase()}</span>
								{/if}
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">{person.display_name}</span>
									<span class="block text-xs text-[var(--color-muted)]">@{person.username}</span>
								</span>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if tab === 'organizations'}
			{#if data.organizations.length === 0}
				<EmptyState icon={Building2} title={t('discover.orgs_empty', 'No organizations yet')} />
			{:else}
				<ul class="space-y-2">
					{#each data.organizations as org (org.id)}
						<li>
							<a href="/org/{org.slug}" class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:bg-[var(--color-surface-subtle)]">
								<span class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">{org.name.charAt(0).toUpperCase()}</span>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">
										{org.name}
										{#if org.verified}<span class="ml-1 text-[var(--color-primary)]" aria-label="verified">✓</span>{/if}
									</span>
									{#if org.tagline}
										<span class="block truncate text-xs text-[var(--color-muted)]">{org.tagline}</span>
									{/if}
								</span>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if tab === 'tags'}
			{#if data.tags.length === 0}
				<EmptyState icon={Hash} title={t('discover.tags_empty', 'No tag suggestions yet')} />
			{:else}
				<ul class="flex flex-wrap gap-2">
					{#each data.tags as tag (tag.id)}
						<li>
							<a href="/tags/{tag.slug}" class="inline-flex items-center gap-1 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]">
								<Hash size={14} />{tag.name}
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if tab === 'universities'}
			{#if data.universities.length === 0}
				<EmptyState icon={GraduationCap} title={t('discover.universities_empty', 'No universities yet')} />
			{:else}
				<ul class="space-y-2">
					{#each data.universities as uni (uni.id)}
						<li>
							<a href="/spaces/{uni.slug}" class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:bg-[var(--color-surface-subtle)]">
								<span class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]"><GraduationCap size={18} /></span>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">{uni.name}</span>
									<span class="block text-xs text-[var(--color-muted)]">{t('meta.members', '{n} members').replace('{n}', String(uni.member_count))}</span>
								</span>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		{/if}
	</div>
</div>

<style>
	.discover-tabs {
		scrollbar-width: none;
	}
	.discover-tabs::-webkit-scrollbar {
		display: none;
	}
</style>
