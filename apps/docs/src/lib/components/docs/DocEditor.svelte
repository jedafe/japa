<script lang="ts">
	// DocEditor — docs surface wrapper around the shared @japaconnect/ui
	// RichTextEditor (the same TipTap editor used by the main app's /create
	// composer). Keeps this app's bind:value JSON contract: the docs
	// backend stores doc_pages.content as TipTap JSON (§44).
	import { RichTextEditor } from '@japaconnect/ui';
	import { t } from '$lib/i18n';

	interface Props {
		/**
		 * Initial content: a TipTap doc JSON object or an HTML string (for
		 * pages whose stored content is markdown/plain — seeded from the
		 * server-rendered HTML so existing formatting survives).
		 */
		initialContent?: unknown;
		/** Bindable output — the current TipTap JSON doc, updated on edit. */
		value?: unknown;
		placeholder?: string;
		/** Called when the user edits (not on seed) — powers dirty tracking. */
		onUpdate?: () => void;
	}

	let {
		initialContent = null,
		value = $bindable<unknown>(null),
		placeholder = '',
		onUpdate
	}: Props = $props();

	let rte: RichTextEditor | undefined = $state(undefined);

	/** Ensure any pending content sync is flushed before the host saves. */
	export function flush() {
		rte?.flush();
	}
</script>

<RichTextEditor
	bind:this={rte}
	content={initialContent}
	{t}
	placeholder={placeholder || t('editor.content_label', 'Page content')}
	ariaLabel={t('editor.content_label', 'Page content')}
	defaultMode="wysiwyg"
	modes={['wysiwyg', 'block']}
	minHeight={320}
	features={{
		image: true,
		youtube: true,
		markdown: false,
		suggestions: false,
		coupon: false,
		linkPreview: false,
		video: false
	}}
	onjson={(json) => {
		value = json;
	}}
	onedit={() => onUpdate?.()}
/>
