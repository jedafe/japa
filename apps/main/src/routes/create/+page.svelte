<script lang="ts">
	import ContentTypeSelector from '$lib/create/ContentTypeSelector.svelte';
	import { MetadataHead } from '$lib/seo';
	import { api } from '$lib/api';
	import { onMount } from 'svelte';

	let contentTypes = $state([
		{ slug: 'blog_post', icon: 'file-text', label: 'Blog Post', description: 'Write a blog post or article' },
		{ slug: 'discussion', icon: 'message-square', label: 'Discussion', description: 'Start a discussion thread' },
		{ slug: 'guide', icon: 'book-open', label: 'Guide', description: 'Create a comprehensive guide' },
		{ slug: 'event', icon: 'calendar', label: 'Event', description: 'Create an event or meetup' },
		{ slug: 'question', icon: 'help-circle', label: 'Question', description: 'Ask a question to the community' },
		{ slug: 'listing', icon: 'clipboard-list', label: 'Listing', description: 'Create a job or service listing' },
		{ slug: 'comparison', icon: 'scale', label: 'Comparison', description: 'Compare two options or services' }
	]);

	onMount(async () => {
		try {
			const types = await api.listContentTypes();
			if (Array.isArray(types) && types.length > 0) {
				contentTypes = types.map((t) => ({
					slug: t.value,
					icon: 'file-text',
					label: t.label,
					description: `Create a ${t.label.toLowerCase()}`
				}));
			}
		} catch {
			// Keep default content types
		}
	});

	const metadata = $derived({
		title: 'Create New Post | JapaConnect',
		description: 'Create a new post on JapaConnect — choose a content type to get started.'
	});
</script>

<MetadataHead {metadata} noindex={true} />

<main class="mx-auto max-w-4xl px-6 py-16">
	<h1 class="text-3xl font-bold tracking-tight">Create New Post</h1>
	<p class="mt-2 text-lg" style="color: var(--color-muted)">
		Choose a content type to get started
	</p>

	<div class="mt-10">
		<ContentTypeSelector types={contentTypes} />
	</div>
</main>
