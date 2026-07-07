<script lang="ts">
	import type { Component } from 'svelte';

	interface Props {
		content?: string;
		onChange?: (json: string) => void;
		readOnly?: boolean;
	}

	let {
		content = '',
		onChange,
		readOnly = false,
	}: Props = $props();

	let EditorComponent: Component | undefined = $state();
	let editorLoaded = $state(false);

	function safeParseHTML(json: string): string {
		if (!json) return '';
		try {
			const parsed = JSON.parse(json);
			if (parsed?.type === 'doc' && Array.isArray(parsed.content)) {
				return json;
			}
		} catch {
			// not JSON — treat as raw HTML
		}
		return json;
	}

	let parsedContent = $derived(safeParseHTML(content));
	let wrapperClass = $derived(
		`block-rich-text${!readOnly ? ' ring-2 ring-blue-400/40 hover:ring-blue-400/60' : ''}`
	);

	// Lazy-load the editor only when not readOnly
	$effect(() => {
		if (!readOnly && !editorLoaded) {
			import('$lib/components/editor/Editor.svelte').then((mod) => {
				EditorComponent = mod.default;
				editorLoaded = true;
			});
		}
	});
</script>

<div class={wrapperClass}>
	{#if readOnly}
		<div class="prose prose-sm max-w-none dark:prose-invert">
			{@html parsedContent}
		</div>
	{:else if EditorComponent}
		<EditorComponent
			{content}
			{onChange}
			readOnly={false}
			placeholder="Write your content..."
		/>
	{:else}
		<div class="flex items-center justify-center p-8 text-sm text-gray-400 dark:text-gray-500">
			Loading editor...
		</div>
	{/if}
</div>
