<script lang="ts">
	/**
	 * User-only hover card convenience wrapper.
	 * Defaults type to 'user' and safely falls back to plain markup
	 * when no username/slug is available.
	 */
	import type { Snippet } from 'svelte';
	import EntityHoverChip from './EntityHoverChip.svelte';

	interface Props {
		id: string;
		slug?: string | null;
		name: string;
		avatar?: string | null;
		variant?: 'chip' | 'row';
		secondary?: string;
		trigger?: Snippet;
		description?: string | null;
		count?: number;
		isFollowing?: boolean;
		class?: string;
	}

	let {
		id,
		slug,
		name,
		avatar,
		variant = 'chip',
		secondary,
		trigger,
		description,
		count,
		isFollowing = false,
		class: classProp
	}: Props = $props();
</script>

<EntityHoverChip
	type="user"
	{id}
	{slug}
	{name}
	{avatar}
	{variant}
	{secondary}
	{description}
	{count}
	{isFollowing}
	class={classProp}
	trigger={trigger}
/>
