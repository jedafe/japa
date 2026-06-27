<script lang="ts">
	import { page } from '$app/state';
	import { ArrowLeft, Clock, Tag } from 'lucide-svelte';
	import DocContent from '$lib/components/DocContent.svelte';

	const { data } = $props<{ data: { doc: { title: string; content: Record<string, unknown>; audience: string; updated_at: string } | null } }>();

	let currentAudience = $derived(page.params.audience ?? '');
	let docPage = $derived(data.doc);

	function renderText(node: Record<string, unknown>): string {
		const text = (node.text as string) ?? '';
		if (!node.marks) return escapeHtml(text);
		let out = '';
		const marks = node.marks as Array<Record<string, unknown>>;
		for (const mark of marks) {
			if (mark.type === 'bold') out += '<strong>';
			if (mark.type === 'italic') out += '<em>';
			if (mark.type === 'code') out += '<code>';
		}
		out += escapeHtml(text);
		for (const mark of marks.slice().reverse()) {
			if (mark.type === 'code') out += '</code>';
			if (mark.type === 'italic') out += '</em>';
			if (mark.type === 'bold') out += '</strong>';
		}
		return out;
	}

	function escapeHtml(s: string): string {
		return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
	}

	function renderNode(node: Record<string, unknown>): string {
		if (!node) return '';
		const nodeType = node.type as string;
		const content = node.content as Array<Record<string, unknown>> | undefined;
		if (nodeType === 'text') return renderText(node);
		const children = (content ?? []).map(renderNode).join('');
		const attrs = (node.attrs ?? {}) as Record<string, unknown>;
		switch (nodeType) {
			case 'doc': return children;
			case 'paragraph': return '<p>' + children + '</p>';
			case 'heading': {
				const level = attrs.level ?? 2;
				return '<h' + level + '>' + children + '</h' + level + '>';
			}
			case 'bullet_list': return '<ul>' + children + '</ul>';
			case 'ordered_list': return '<ol>' + children + '</ol>';
			case 'list_item': return '<li>' + children + '</li>';
			case 'code_block': return '<pre><code>' + children + '</code></pre>';
			case 'blockquote': return '<blockquote>' + children + '</blockquote>';
			case 'horizontal_rule': return '<hr />';
			case 'image':
				return '<img src="' + String(attrs.src) + '" alt="' + String(attrs.alt ?? '') + '" />';
			default: return children;
		}
	}
</script>

<svelte:head>
	<title>{docPage?.title ?? 'Not Found'} — JapaConnect Docs</title>
</svelte:head>

<div class="mx-auto max-w-3xl">
	{#if docPage}
		<a href="/{currentAudience}" class="mb-6 inline-flex items-center gap-1 text-sm hover:underline" style="color: var(--color-muted)">
			<ArrowLeft class="h-3 w-3" />
			Back to docs
		</a>

		<h1 class="text-3xl font-bold tracking-tight">{docPage.title}</h1>

		<div class="mt-2 flex items-center gap-4 text-xs" style="color: var(--color-muted)">
			<span class="flex items-center gap-1">
				<Clock class="h-3 w-3" />
				Updated {new Date(docPage.updated_at).toLocaleDateString()}
			</span>
			<span class="flex items-center gap-1">
				<Tag class="h-3 w-3" />
				{docPage.audience}
			</span>
		</div>

		<div class="mt-8">
			<DocContent>
				{@html renderNode(docPage.content)}
			</DocContent>
		</div>
	{:else}
		<h1 class="text-3xl font-bold tracking-tight">Page Not Found</h1>
		<p class="mt-4" style="color: var(--color-muted)">The documentation page you're looking for doesn't exist or isn't published yet.</p>
	{/if}
</div>
