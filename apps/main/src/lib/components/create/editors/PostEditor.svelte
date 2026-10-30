<script lang="ts" module>
	export type { EditorMode } from '@japaconnect/ui';
</script>

<script lang="ts">
	// PostEditor — main-app wrapper around the shared @japaconnect/ui
	// RichTextEditor. All app-specific integrations live here: mention /
	// hashtag / interlink / literal search, coupon search, the media
	// library image picker, and link-preview fetching.
	// Ref: japaconnect-engineering/sections/15-tiptap-rich-text.md
	import { RichTextEditor } from '@japaconnect/ui';
	import type { EditorMode, LinkPreviewData } from '@japaconnect/ui';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { searchHits } from '$lib/api/create';
	import type { MediaAsset } from '$lib/types';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';

	interface Props {
		html: string;
		onedit: () => void;
		onhtml?: (html: string) => void;
		/** Fires with the editor's TipTap JSON document on every update.
		 *  Used by the forum composer which stores body as base64 TipTap JSON. */
		onjson?: (json: Record<string, unknown>) => void;
		onfocus?: () => void;
		defaultMode?: EditorMode;
		aiTrigger?: import('svelte').Snippet;
		/** Current post title — read live when the FAQ block seeds its
		 *  auto-generated subheading. */
		postTitle?: string;
	}

	let {
		html,
		onedit,
		onhtml,
		onjson,
		onfocus,
		defaultMode = 'block',
		aiTrigger,
		postTitle
	}: Props = $props();

	let rte = $state.raw<RichTextEditor | undefined>(undefined);

	// ---- Media picker (image upload via MediaLibraryModal) ----
	let mediaPickerOpen = $state(false);
	let pendingImageInsert = $state<((src: string, alt?: string) => void) | null>(null);

	function handleMediaSelect(asset: MediaAsset) {
		mediaPickerOpen = false;
		pendingImageInsert?.(asset.url, asset.alt_text ?? asset.filename ?? '');
		pendingImageInsert = null;
	}

	async function fetchLinkPreview(url: string): Promise<LinkPreviewData | null> {
		const result = await api.get<LinkPreviewData>(
			`/link-preview?url=${encodeURIComponent(url)}`
		);
		return result.data ?? null;
	}

	export function getHTML(): string {
		return rte?.getHTML() ?? '';
	}

	export function getJSON(): Record<string, unknown> {
		return rte?.getJSON() ?? { type: 'doc', content: [] };
	}

	/** Flush any pending markdown → doc sync before the host reads state. */
	export function flush() {
		rte?.flush();
	}

	/**
	 * Append a TipTap node (or a whole `{type:'doc',content:[…]}` document's
	 * content) at the end of the editor. Used by the video composer's
	 * VideoField to insert the backend-built `video` node after a URL
	 * resolve or file upload (§106).
	 */
	export function appendVideoNode(json: Record<string, unknown>) {
		rte?.appendVideoNode(json);
	}
</script>

<RichTextEditor
	bind:this={rte}
	content={html}
	{t}
	{onedit}
	{onhtml}
	{onjson}
	{onfocus}
	{defaultMode}
	{aiTrigger}
	{postTitle}
	placeholder={t('create.editor.placeholder')}
	onSearch={searchHits}
	onSearchCoupons={(q) => searchHits('coupons', q)}
	{fetchLinkPreview}
	onPickImage={(insert) => {
		pendingImageInsert = insert;
		mediaPickerOpen = true;
	}}
	features={{
		suggestions: true,
		coupon: true,
		linkPreview: true,
		video: true,
		image: true,
		youtube: true,
		markdown: true
	}}
/>

{#if mediaPickerOpen}
	<MediaLibraryModal onSelect={handleMediaSelect} onClose={() => (mediaPickerOpen = false)} />
{/if}
