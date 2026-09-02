<script lang="ts">
	/**
	 * Reusable tab bar built on Bits UI Tabs, styled for the JCE surface.
	 * Ref: japaconnect-engineering/conventions/svelte-component-patterns.md
	 */
	import { Tabs } from 'bits-ui';

	export interface JceTab {
		value: string;
		label: string;
	}

	interface Props {
		tabs: JceTab[];
		value: string;
		onValueChange?: (value: string) => void;
		class?: string;
	}

	let { tabs, value, onValueChange, class: className = '' }: Props = $props();

	function handleChange(newValue: string | undefined) {
		if (newValue !== undefined) {
			onValueChange?.(newValue);
		}
	}
</script>

<Tabs.Root {value} onValueChange={handleChange}>
	<Tabs.List
		class="flex gap-1 overflow-x-auto overflow-y-hidden border-b {className}"
		style="border-color: var(--color-border)"
	>
		{#each tabs as tab (tab.value)}
			<Tabs.Trigger
				value={tab.value}
				class="relative flex shrink-0 cursor-pointer items-center px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-[1px] data-[state=active]:border-[var(--color-text)] data-[state=active]:text-[var(--color-text)] data-[state=inactive]:border-transparent data-[state=inactive]:text-[var(--color-muted)] data-[state=inactive]:hover:text-[var(--color-text)]"
			>
				{#snippet child({ props })}
					<span {...props}>{tab.label}</span>
				{/snippet}
			</Tabs.Trigger>
		{/each}
	</Tabs.List>
</Tabs.Root>
