<script lang="ts">
	import { page } from '$app/stores';
	import PostCreateLayout from '$lib/create/PostCreateLayout.svelte';

	const contentTypes: Record<string, { slug: string; label: string; icon: string }> = {
		blog_post: { slug: 'blog_post', label: 'Blog Post', icon: 'file-text' },
		discussion: { slug: 'discussion', label: 'Discussion', icon: 'message-square' },
		guide: { slug: 'guide', label: 'Guide', icon: 'book-open' },
		event: { slug: 'event', label: 'Event', icon: 'calendar' },
		question: { slug: 'question', label: 'Question', icon: 'help-circle' },
		listing: { slug: 'listing', label: 'Listing', icon: 'clipboard-list' },
		comparison: { slug: 'comparison', label: 'Comparison', icon: 'scale' }
	};

	let contentType = $derived($page.params.content_type ? contentTypes[$page.params.content_type] : undefined);
	let pageTitle = $derived(contentType ? `Create ${contentType.label} | JapaConnect` : 'Content Type Not Found | JapaConnect');

	function handleSave(data: { title: string; content: string; metadata: Record<string, unknown> }) {
		console.log('Saving draft:', data);
	}

	function handlePublish(data: { title: string; content: string; metadata: Record<string, unknown> }) {
		console.log('Publishing:', data);
	}
</script>

<svelte:head>
	<title>{pageTitle}</title>
</svelte:head>

{#if contentType}
	<PostCreateLayout {contentType} onSave={handleSave} onPublish={handlePublish} />
{:else}
	<div class="flex min-h-screen items-center justify-center">
		<div class="text-center">
			<h1 class="text-2xl font-bold">Content type not found</h1>
			<p class="mt-2" style="color: var(--color-muted)">
				The content type "{$page.params.content_type}" does not exist.
			</p>
			<a
				href="/create"
				class="mt-6 inline-block rounded-md px-5 py-2.5 text-sm font-medium text-white transition-colors"
				style="background: var(--color-primary)"
			>
				Choose another type
			</a>
		</div>
	</div>
{/if}
