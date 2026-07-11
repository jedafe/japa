<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import PostCreateLayout from '$lib/create/PostCreateLayout.svelte';
	import { api } from '$lib/api';
	import { onMount } from 'svelte';

	let contentTypes = $state<Record<string, { slug: string; label: string; icon: string }>>({});

	onMount(async () => {
		try {
			const types = await api.listContentTypes();
			if (Array.isArray(types) && types.length > 0) {
				const mapped: Record<string, { slug: string; label: string; icon: string }> = {};
				for (const t of types) {
					mapped[t.value] = { slug: t.value, label: t.label, icon: 'file-text' };
				}
				contentTypes = mapped;
			}
		} catch {
			contentTypes = {
				blog_post: { slug: 'blog_post', label: 'Blog Post', icon: 'file-text' },
				discussion: { slug: 'discussion', label: 'Discussion', icon: 'message-square' },
				guide: { slug: 'guide', label: 'Guide', icon: 'book-open' },
				event: { slug: 'event', label: 'Event', icon: 'calendar' },
				question: { slug: 'question', label: 'Question', icon: 'help-circle' },
				listing: { slug: 'listing', label: 'Listing', icon: 'clipboard-list' },
				comparison: { slug: 'comparison', label: 'Comparison', icon: 'scale' }
			};
		}
	});

	let contentType = $derived(page.params.content_type ? contentTypes[page.params.content_type] : undefined);
	let pageTitle = $derived(contentType ? `Create ${contentType.label} | JapaConnect` : 'Content Type Not Found | JapaConnect');

	let saveError = $state<string | null>(null);
	let lastSavedId = $state<string | null>(null);

	async function handleSave(data: { title: string; content: string; metadata: Record<string, unknown> }) {
		const ct = page.params.content_type;
		if (!ct) return;
		saveError = null;
		try {
			if (lastSavedId) {
				await api.updateContent(lastSavedId, {
					title: data.title,
					content: data.content,
					field_values: data.metadata
				});
			} else {
				const result = await api.createContent({
					content_type: ct,
					title: data.title,
					content: data.content,
					status: 'draft',
					field_values: data.metadata
				});
				lastSavedId = result.id;
			}
		} catch (e) {
			saveError = e instanceof Error ? e.message : 'Failed to save';
		}
	}

	async function handlePublish(data: { title: string; content: string; metadata: Record<string, unknown> }) {
		const ct = page.params.content_type;
		if (!ct) return;
		saveError = null;
		try {
			if (lastSavedId) {
				await api.updateContent(lastSavedId, {
					title: data.title,
					content: data.content,
					field_values: data.metadata,
					status: 'published'
				});
			} else {
				await api.createContent({
					content_type: ct,
					title: data.title,
					content: data.content,
					status: 'published',
					field_values: data.metadata
				});
			}
			goto('/');
		} catch (e) {
			saveError = e instanceof Error ? e.message : 'Failed to publish';
		}
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
				The content type "{page.params.content_type}" does not exist.
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
