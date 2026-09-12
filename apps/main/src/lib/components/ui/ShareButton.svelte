<script lang="ts">
	import { Share2 } from 'lucide-svelte';

	interface Props {
		url: string;
		title?: string;
		class?: string;
		onShared?: () => void;
	}

	let {
		url,
		title,
		class: classProp,
		onShared
	}: Props = $props();

	async function onClick(e: Event) {
		e.stopPropagation();
		try {
			if (navigator.share) {
				await navigator.share({ title: title ?? '', url });
			} else if (navigator.clipboard) {
				await navigator.clipboard.writeText(url);
			}
			onShared?.();
		} catch {
			// user cancelled or unavailable
		}
	}
</script>

<button
	type="button"
	onclick={onClick}
	class={classProp ??
		'grid h-9 w-9 place-items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-[var(--color-text)] transition hover:bg-[var(--color-surface-hover)]'}
	aria-label={title ? `Share ${title}` : 'Share'}
>
	<Share2 size={18} aria-hidden="true" />
</button>
