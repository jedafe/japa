<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { BookOpen, ChevronRight, FileText, Search } from 'lucide-svelte';

	let { audience, currentSlug = '' }: { audience: string; currentSlug?: string } = $props();

	const sections = [
		{ slug: 'users', labelKey: 'sections.user_guide', icon: BookOpen },
		{ slug: 'premium', labelKey: 'sections.premium', icon: ChevronRight },
		{ slug: 'organizations', labelKey: 'sections.organizations', icon: ChevronRight },
		{ slug: 'moderators', labelKey: 'sections.moderators', icon: FileText },
		{ slug: 'api', labelKey: 'sections.api_reference', icon: FileText },
		{ slug: 'admin', labelKey: 'sections.admin', icon: FileText },
	];
</script>

<aside class="w-64 shrink-0 border-r p-4" style="border-color: var(--color-border)">
	<!-- Brand -->
	<div class="mb-6">
		<a href="/" class="text-lg font-bold tracking-tight" style="color: var(--color-primary)">
			{t('docs.brand')}
		</a>
	</div>

	<!-- Search placeholder -->
	<div class="relative mb-6">
		<Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
		<input
			type="text"
			placeholder={t('sidebar.search')}
			class="w-full rounded-md border py-2 pl-9 pr-3 text-sm outline-none focus:ring-2"
			style="border-color: var(--color-border); background: var(--color-surface)"
		/>
	</div>

	<!-- Navigation -->
	<nav class="space-y-1">
		{#each sections as section}
			{@const Icon = section.icon}
			<a
				href="/{section.slug}"
				class="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors"
				class:bg-blue-50={audience === section.slug}
				class:text-blue-700={audience === section.slug}
				style={audience !== section.slug ? 'color: var(--color-muted)' : ''}
			>
				<Icon class="h-4 w-4" />
				{t(section.labelKey)}
			</a>
		{/each}
	</nav>
</aside>
