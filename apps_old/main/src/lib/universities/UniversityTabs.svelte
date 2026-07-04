<script lang="ts">
	import { FileText, MessageSquare, BookOpen, Users, Star, Scale } from 'lucide-svelte';

	let {
		activeTab = $bindable('overview'),
		tabs = ['overview', 'posts', 'forums', 'courses', 'members', 'reviews', 'compare']
	}: {
		activeTab?: string;
		tabs?: string[];
	} = $props();

	const tabConfig: Record<string, { label: string; icon: any }> = {
		overview: { label: 'Overview', icon: FileText },
		posts: { label: 'Posts', icon: FileText },
		forums: { label: 'Forums', icon: MessageSquare },
		courses: { label: 'Courses', icon: BookOpen },
		members: { label: 'Members', icon: Users },
		reviews: { label: 'Reviews', icon: Star },
		compare: { label: 'Compare', icon: Scale }
	};

	function selectTab(tab: string) {
		activeTab = tab;
	}
</script>

<nav class="flex gap-1 overflow-x-auto border-b" style="border-color: var(--color-border)">
	{#each tabs as tab}
		{@const config = tabConfig[tab]}
		{#if config}
			<button
				onclick={() => selectTab(tab)}
				class="flex items-center gap-2 whitespace-nowrap border-b-2 px-4 py-3 text-sm font-medium transition-colors"
				style="border-color: {activeTab === tab ? 'var(--color-primary)' : 'transparent'}; color: {activeTab === tab ? 'var(--color-primary)' : 'var(--color-muted)'}"
			>
				<config.icon class="h-4 w-4" />
				{config.label}
			</button>
		{/if}
	{/each}
</nav>
