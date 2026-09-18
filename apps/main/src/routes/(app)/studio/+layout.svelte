<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md — /studio tab strip.
	// Renders on every subroute including /studio/create (ADR-058 Exception #2
	// keeps the rails hidden; the strip is the only chrome above the composer).
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { Clapperboard, FolderOpen, Video, Wand2, PenSquare } from 'lucide-svelte';
	import type { IconComponent } from '$lib/components/ui/icons';

	type TabDef = {
		id: string;
		href: string;
		labelKey: string;
		fallback: string;
		icon: IconComponent;
		disabled?: boolean;
		disabledKey?: string;
		disabledFallback?: string;
	};

	let { data, children } = $props();

	const TABS = $derived<TabDef[]>([
		{
			id: 'contents',
			href: '/studio/contents',
			labelKey: 'studio.tabs.contents',
			fallback: 'Contents',
			icon: FolderOpen,
			disabled: true,
			disabledKey: 'studio.tabs.coming_soon',
			disabledFallback: 'Coming soon'
		},
		{
			id: 'videos',
			href: '/studio/videos',
			labelKey: 'studio.tabs.videos',
			fallback: 'Videos',
			icon: Video,
			disabled: !data.videoEnabled,
			disabledKey: 'studio.tabs.videos_disabled',
			disabledFallback: 'Video is not enabled on this site'
		},
		{
			id: 'prompts',
			href: '/studio/prompts',
			labelKey: 'studio.tabs.prompts',
			fallback: 'Prompts',
			icon: Wand2
		},
		{
			id: 'create',
			href: '/studio/create',
			labelKey: 'studio.tabs.create',
			fallback: 'Create',
			icon: PenSquare
		}
	]);

	const activeTab = $derived(page.url.pathname.split('/studio/')[1]?.split('/')[0] ?? '');
</script>

<svelte:head>
	<title>{t('studio.title', 'Studio — JapaConnect')}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="border-b border-[var(--color-border)] px-4 pt-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Clapperboard size={20} /> {t('studio.heading', 'Studio')}
		</h1>

		<nav class="mt-2 flex gap-1" aria-label={t('studio.tabs.label', 'Studio sections')}>
			{#each TABS as tab (tab.id)}
				{@const active = activeTab === tab.id}
				{#if tab.disabled}
					<span
						title={t(tab.disabledKey ?? '', tab.disabledFallback ?? '')}
						class="flex cursor-not-allowed items-center gap-2 border-b-2 border-transparent px-3 py-2 text-sm font-medium text-[var(--color-muted)] opacity-50"
						aria-disabled="true"
					>
						<tab.icon size={16} /> {t(tab.labelKey, tab.fallback)}
					</span>
				{:else}
					<a
						href={tab.href}
						aria-current={active ? 'page' : undefined}
						class="flex items-center gap-2 border-b-2 px-3 py-2 text-sm font-medium transition-colors
							{active
							? 'border-[var(--color-primary)] text-[var(--color-text)]'
							: 'border-transparent text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					>
						<tab.icon size={16} /> {t(tab.labelKey, tab.fallback)}
					</a>
				{/if}
			{/each}
		</nav>
	</div>

	{@render children()}
</div>
