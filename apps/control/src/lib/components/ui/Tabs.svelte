<script lang="ts" module>
	import type { Snippet } from 'svelte';
	import type { IconComponent } from '@japaconnect/ui';

	export interface TabItem {
		value: string;
		label: string | Snippet;
		icon?: IconComponent;
		badge?: string | number;
	}
</script>

<script lang="ts">
	import { Tabs } from 'bits-ui';

	interface Props {
		value?: string;
		tabs: TabItem[];
		onValueChange?: (value: string) => void;
		class?: string;
		/** Full override for the tab list classes (replaces the defaults). */
		listClass?: string;
		/** Full override for each trigger's classes (replaces the defaults). */
		triggerClass?: string;
		children?: Snippet;
	}

	let {
		value = $bindable(''),
		tabs,
		onValueChange,
		class: className = '',
		listClass = '',
		triggerClass = '',
		children
	}: Props = $props();
</script>

<Tabs.Root bind:value onValueChange={(v) => onValueChange?.(v)} class={className}>
	<Tabs.List class={listClass || 'bg-muted rounded-9px inline-flex gap-1 p-1'}>
		{#each tabs as tab (tab.value)}
			<Tabs.Trigger
				value={tab.value}
				class={triggerClass ||
					'data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all'}
			>
				{#if tab.icon}
					{@const TabIcon = tab.icon}
					<TabIcon class="h-3.5 w-3.5" />
				{/if}
				{#if typeof tab.label === 'string'}
					{tab.label}
				{:else}
					{@render tab.label()}
				{/if}
				{#if tab.badge !== undefined}
					<span class="bg-accent text-accent-foreground ms-1.5 rounded-full px-1.5 py-0.5 text-[10px] font-medium">
						{tab.badge}
					</span>
				{/if}
			</Tabs.Trigger>
		{/each}
	</Tabs.List>
	{@render children?.()}
</Tabs.Root>
