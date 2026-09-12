<script lang="ts">
	import { DropdownMenu } from 'bits-ui';
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

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		type="button"
		class={classProp ??
			'grid h-9 w-9 place-items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-[var(--color-text)] transition hover:bg-[var(--color-surface-hover)]'}
		aria-label={title ? t('feed.card.share', 'Share {title}').replace('{title}', title) : t('feed.card.share', 'Share')}
	>
		{#if sharedOrCopied}
			<Check {size} class={copied ? 'text-[var(--color-success)]' : ''} aria-hidden="true" />
		{:else}
			<Share2 {size} aria-hidden="true" />
		{/if}
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
			{#if contentId}
				<ShareMenuItems {contentId} {url} title={title ?? ''} onShared={onShared} />
			{/if}

			<DropdownMenu.Item
				onclick={contentId ? nativeShare : copyOnly}
				class="flex w-full cursor-pointer items-center gap-2 rounded px-3 py-2 text-sm text-[var(--color-text)] transition hover:bg-[var(--color-surface-hover)]"
			>
				<Share2 size={14} />
				{contentId ? t('blog.copy_link', 'Copy link / Native share') : t('blog.copy_link', 'Copy link')}
			</DropdownMenu.Item>
		</DropdownMenu.Content>
	</DropdownMenu.Portal>
</DropdownMenu.Root>
