<script lang="ts">
	import { DropdownMenu } from 'bits-ui';
	import { Share2, Check } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import ShareMenuItems from './ShareMenuItems.svelte';

	interface Props {
		contentId: string;
		url: string;
		title: string;
		/** Optional shared/copied transient state. */
		size?: number;
	}
	let { contentId, url, title, size = 14 }: Props = $props();

	let shared = $state(false);

	function onShared() {
		shared = true;
		setTimeout(() => (shared = false), 2000);
	}
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		class="group/btn flex items-center transition-colors"
		aria-label={t('feed.card.share', 'Share')}
	>
		<span
			class="rounded-full p-2 transition-colors group-hover/btn:bg-[var(--color-primary)]/10 group-hover/btn:text-[var(--color-primary)]"
		>
			{#if shared}
				<Check {size} class="text-[var(--color-success)]" />
			{:else}
				<Share2 {size} />
			{/if}
		</span>
	</DropdownMenu.Trigger>
	<DropdownMenu.Portal>
		<DropdownMenu.Content
			align="end"
			sideOffset={8}
			class="z-50 min-w-48 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
		>
			<div class="mb-1 px-3 py-1.5 text-xs font-semibold uppercase text-[var(--color-muted)]">
				{t('feed.share_to', 'Share to')}
			</div>
			<ShareMenuItems {contentId} {url} {title} onShared={onShared} />
		</DropdownMenu.Content>
	</DropdownMenu.Portal>
</DropdownMenu.Root>
