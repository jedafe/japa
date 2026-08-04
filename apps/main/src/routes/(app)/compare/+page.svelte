<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { GitCompare, Plus, MessageSquare, Eye, Lock, Globe, Link2 } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { ComparisonListItem } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const user = $derived(page.data.user);
	const comparisons = $derived<ComparisonListItem[]>(data.comparisons);

	const title = $derived(t('compare.title', 'Compare — JapaConnect'));
	const description = $derived(
		t('compare.description', 'Browse and create comparisons to evaluate items across dimensions.')
	);

	function visibilityChip(v: string) {
		if (v === 'private') return { icon: Lock, label: t('compare.visibility_private', 'Private') };
		if (v === 'unlisted') return { icon: Link2, label: t('compare.visibility_unlisted', 'Unlisted') };
		return { icon: Globe, label: t('compare.visibility_public', 'Public') };
	}

	function excerpt(body: string, max = 140) {
		const text = body.replace(/<[^>]*>/g, '').trim();
		return text.length > max ? text.slice(0, max) + '…' : text;
	}

	function formatDate(iso: string | null) {
		if (!iso) return '';
		const d = new Date(iso);
		return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
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

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<GitCompare size={20} /> {t('compare.heading', 'Comparisons')}
		</h1>
		{#if user}
			<a
				href="/compare/new"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<Plus size={16} /> {t('compare.new_short', 'New')}
			</a>
		{/if}
	</div>

	<div class="p-3">
		{#if comparisons.length === 0}
			<EmptyState
				icon={GitCompare}
				title={t('compare.empty_title', 'No comparisons yet')}
				body={t(
					'compare.empty_body',
					'Comparisons help you evaluate items side by side across dimensions.'
				)}
				action={user
					? { label: t('compare.new_heading', 'New comparison'), href: '/compare/new' }
					: { label: t('common.sign_in', 'Sign in'), href: '/login?redirect=/compare' }}
			/>
		{:else}
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each comparisons as cmp (cmp.id)}
					{@const chip = visibilityChip(cmp.visibility)}
					<a
						href="/compare/{cmp.id}"
						class="group flex flex-col gap-2 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]"
					>
						<div class="flex items-start justify-between gap-2">
							<h2 class="line-clamp-2 text-sm font-semibold text-[var(--color-text)] group-hover:text-[var(--color-primary)]">
								{cmp.title}
							</h2>
							<span
								class="inline-flex shrink-0 items-center gap-1 rounded-full border border-[var(--color-border)] px-2 py-0.5 text-[10px] font-medium uppercase text-[var(--color-muted)]"
							>
								<chip.icon size={11} /> {chip.label}
							</span>
						</div>

						{#if excerpt(cmp.body)}
							<p class="line-clamp-2 text-xs text-[var(--color-text-subtle)]">{excerpt(cmp.body)}</p>
						{/if}

						<div class="mt-auto flex items-center gap-3 pt-2 text-xs text-[var(--color-muted)]">
							<span class="inline-flex items-center gap-1">
								<Eye size={12} /> {cmp.view_count}
							</span>
							<span class="inline-flex items-center gap-1">
								<MessageSquare size={12} /> {cmp.comment_count}
							</span>
							{#if formatDate(cmp.published_at ?? cmp.created_at)}
								<span class="ml-auto">{formatDate(cmp.published_at ?? cmp.created_at)}</span>
							{/if}
						</div>
					</a>
				{/each}
			</div>
		{/if}
	</div>
</div>
