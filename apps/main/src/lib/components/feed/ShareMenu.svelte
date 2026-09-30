<script lang="ts">
	import { Menu } from '$lib/components/ui';
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

<Menu>
	{#snippet trigger(props)}
		<button
			type="button"
			{...props}
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
		</button>
	{/snippet}
	<div class="mb-1 px-3 py-1.5 text-xs font-semibold uppercase text-[var(--color-muted)]">
		{t('feed.share_to', 'Share to')}
	</div>
	<ShareMenuItems {contentId} {url} {title} onShared={onShared} />
</Menu>
