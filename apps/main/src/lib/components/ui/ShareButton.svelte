<script lang="ts">
	import { Menu, MenuItem } from '$lib/components/ui';
	import { Share2, Check } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import ShareMenuItems from '$lib/components/feed/ShareMenuItems.svelte';

	interface Props {
		contentId?: string;
		url: string;
		title?: string;
		class?: string;
		size?: number;
	}

	let { contentId, url, title, class: classProp, size = 18 }: Props = $props();

	let shared = $state(false);
	let copied = $state(false);

	function onShared() {
		shared = true;
		setTimeout(() => (shared = false), 2000);
	}

	async function nativeShare() {
		try {
			if (navigator.share) {
				await navigator.share({ title: title ?? '', url });
			} else if (navigator.clipboard) {
				await navigator.clipboard.writeText(url);
				copied = true;
				setTimeout(() => (copied = false), 2000);
			}
			onShared();
		} catch {
			// user cancelled or unavailable
		}
	}

	async function copyOnly() {
		try {
			await navigator.clipboard.writeText(url);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			// Clipboard failed — silently ignore
		}
	}

	const sharedOrCopied = $derived(shared || copied);
</script>

<Menu>
	{#snippet trigger(props)}
		<button
			type="button"
			{...props}
			class={classProp ??
				'grid h-9 w-9 place-items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-[var(--color-text)] transition hover:bg-[var(--color-surface-hover)]'}
			aria-label={title ? t('feed.card.share', 'Share {title}').replace('{title}', title) : t('feed.card.share', 'Share')}
		>
			{#if sharedOrCopied}
				<Check {size} class={copied ? 'text-[var(--color-success)]' : ''} aria-hidden="true" />
			{:else}
				<Share2 {size} aria-hidden="true" />
			{/if}
		</button>
	{/snippet}

	<div class="mb-1 px-3 py-1.5 text-xs font-semibold uppercase text-[var(--color-muted)]">
		{t('feed.share_to', 'Share to')}
	</div>
	{#if contentId}
		<ShareMenuItems {contentId} {url} title={title ?? ''} onShared={onShared} />
	{/if}

	<MenuItem icon={Share2} onSelect={contentId ? nativeShare : copyOnly}>
		{contentId ? t('blog.copy_link', 'Copy link / Native share') : t('blog.copy_link', 'Copy link')}
	</MenuItem>
</Menu>
