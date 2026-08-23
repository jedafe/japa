<script lang="ts">
	// Ref: japaconnect-engineering/sections/05-spaces.md (user's joined spaces)
	// Lists the spaces the user has joined or moderates. The "Browse spaces"
	// CTA routes to /spaces.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { EmptyState, UrlTabs, type UrlTab } from '$lib/components/ui';
	import { UsersRound, Plus } from 'lucide-svelte';
	import type { MeSpace } from '$lib/types';

	let { data } = $props();
	const spaces = $derived(data.spaces);
	const tab = $derived(data.tab ?? 'joined');

	const tabs = $derived<UrlTab[]>([
		{ value: 'joined', label: t('spaces.tab_joined', 'Joined') },
		{ value: 'moderated', label: t('spaces.tab_moderated', 'Moderated') }
	]);
</script>

<section class="space-y-4">
	<header class="space-y-4">
		<div class="flex items-center justify-between">
			<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.spaces')}</h1>
			<a
				href="/spaces"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				<Plus class="h-4 w-4" />
				{t('spaces.browse')}
			</a>
		</div>
		<UrlTabs {tabs} value={tab} onValueChange={(v) => goto(`?tab=${v}`)} />
	</header>

	{#if spaces.length === 0}
		<EmptyState
			icon={UsersRound}
			title={t('spaces.empty_title')}
			body={t('spaces.empty_body')}
		/>
	{:else}
		<ul class="space-y-2">
			{#each spaces as s (s.id)}
				<li>
					<a
						href="/spaces/{s.slug}"
						class="flex items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition hover:border-[var(--color-primary)]/40"
					>
						<div class="min-w-0 flex-1">
							<p class="truncate text-sm font-medium text-[var(--color-text)]">{s.name}</p>
							<p class="mt-0.5 text-xs text-[var(--color-muted)]">/{s.slug}</p>
						</div>
						{#if s.is_primary}
							<span class="shrink-0 rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-medium text-[var(--color-primary)]">
								{t('spaces.primary')}
							</span>
						{/if}
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</section>
