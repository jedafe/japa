<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { GraduationCap, Users } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('universities.title', 'Universities — JapaConnect'));
	const description = $derived(
		t('universities.description', 'Explore universities relevant to the Nigerian diaspora community.')
	);
	// JSON-LD: ItemList of EducationalOrganization (H3 — audit finding).
	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'ItemList',
			itemListElement: data.universities.map((uni, i) => ({
				'@type': 'ListItem',
				position: i + 1,
				item: { '@type': 'EducationalOrganization', name: uni.name, url: `${page.url.origin}/universities/${uni.slug}` }
			}))
		})
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<GraduationCap size={20} /> {t('universities.heading', 'Universities')}
		</h1>
	</div>

	<div class="p-3">
		{#if data.universities.length === 0}
			<EmptyState icon={GraduationCap} title={t('universities.empty', 'No universities yet')} />
		{:else}
			<div class="grid gap-3 sm:grid-cols-2">
				{#each data.universities as uni (uni.id)}
					<a
						href="/universities/{uni.slug}"
						class="flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<span class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]">
							<GraduationCap size={22} />
						</span>
						<span class="min-w-0 flex-1">
							<span class="block truncate text-sm font-medium text-[var(--color-text)]">{uni.name}</span>
							<span class="mt-1 flex items-center gap-3 text-xs text-[var(--color-muted)]">
								<span class="flex items-center gap-1"><Users size={12} /> {uni.member_count}</span>
								<span>{uni.post_count} {t('universities.posts', 'posts')}</span>
							</span>
						</span>
					</a>
				{/each}
			</div>
		{/if}
	</div>
</div>
