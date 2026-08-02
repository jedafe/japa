<script lang="ts">
	// Ref: japaconnect-engineering/sections/34-media.md (user media library)
	// Grid of the user's uploaded media. Image/video assets show a thumbnail;
	// other types show a file icon. Each card shows filename, size, and date.
	import { t } from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Image as ImageIcon, File } from 'lucide-svelte';
	import type { MediaAsset } from '$lib/types';

	let { data } = $props();
	const media = $derived(data.media);

	function isImage(m: MediaAsset): boolean {
		return (m.mime_type ?? '').startsWith('image/');
	}

	function isVideo(m: MediaAsset): boolean {
		return (m.mime_type ?? '').startsWith('video/');
	}

	function formatSize(bytes?: number): string {
		if (!bytes) return '—';
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}
</script>

<section class="space-y-4">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.media')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('media.subtitle')}</p>
	</header>

	{#if media.length === 0}
		<EmptyState
			icon={ImageIcon}
			title={t('media.empty_title')}
			body={t('media.empty_body')}
		/>
	{:else}
		<ul class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
			{#each media as m (m.id)}
				<li class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					<div class="flex aspect-square items-center justify-center bg-[var(--color-surface-subtle)]">
						{#if isImage(m)}
							<img src={m.url} alt={m.alt_text ?? m.filename ?? ''} class="h-full w-full object-cover" loading="lazy" />
						{:else if isVideo(m)}
							<video src={m.url} class="h-full w-full object-cover" muted></video>
						{:else}
							<File class="h-8 w-8 text-[var(--color-muted)]" />
						{/if}
					</div>
					<div class="p-2">
						<p class="truncate text-xs font-medium text-[var(--color-text)]">{m.filename ?? m.id.slice(0, 8)}</p>
						<p class="mt-0.5 text-xs text-[var(--color-muted)]">{formatSize(m.file_size)}</p>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</section>
