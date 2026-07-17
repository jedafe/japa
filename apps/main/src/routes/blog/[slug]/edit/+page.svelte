<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import PostCreateLayout from '$lib/create/PostCreateLayout.svelte';
	import { api } from '$lib/api';
	import { ArrowLeft } from 'lucide-svelte';

	let { data } = $props();
	let post = $derived(data.post);

	let saveError = $state<string | null>(null);

	async function handleSave(d: { title: string; content: string; metadata: Record<string, unknown> }) {
		if (!post) return;
		saveError = null;
		try {
			await api.updateContent(post.id, {
				title: d.title,
				body: d.content,
				field_values: d.metadata
			});
		} catch (e) {
			saveError = e instanceof Error ? e.message : 'Failed to save';
		}
	}

	async function handlePublish(d: { title: string; content: string; metadata: Record<string, unknown> }) {
		if (!post) return;
		saveError = null;
		try {
			await api.updateContent(post.id, {
				title: d.title,
				body: d.content,
				field_values: d.metadata,
				status: 'published'
			});
			goto('/blog/' + post.slug);
		} catch (e) {
			saveError = e instanceof Error ? e.message : 'Failed to publish';
		}
	}
</script>

<svelte:head>
	<title>Edit: {post.title} | JapaConnect Blog</title>
</svelte:head>

<main class="mx-auto max-w-4xl px-4 py-8">
	<a
		href="/blog/{post.slug}"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to post
	</a>

	{#if saveError}
		<div class="mb-4 rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/30 dark:text-red-400">
			{saveError}
		</div>
	{/if}

	<PostCreateLayout
		contentType={{ slug: 'blog_post', label: 'Blog Post', icon: 'file-text' }}
		initialTitle={post.title}
		initialContent={post.body ?? ''}
		onSave={handleSave}
		onPublish={handlePublish}
	/>
</main>
