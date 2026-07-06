<script lang="ts">
	import Editor from '$lib/components/editor/Editor.svelte';

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
</script>

<div class={wrapperClass}>
	{#if readOnly}
		<div class="prose prose-sm max-w-none dark:prose-invert">
			{@html parsedContent}
		</div>
	{:else}
		<Editor
			{content}
			{onChange}
			readOnly={false}
			placeholder="Write your content..."
		/>
	{/if}
</div>
