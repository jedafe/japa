<script lang="ts">
	import { Image, Upload, Trash2, Loader2, Eye, X } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api, type MediaItem } from '$lib/api';

	let { data } = $props();

	let media = $state(data.media);
	let uploading = $state(false);
	let deleting = $state<string | null>(null);
	let errorMessage = $state('');
	let fileInput = $state<HTMLInputElement>();
	let previewItem = $state<MediaItem | null>(null);

	function formatFileSize(bytes: number): string {
		if (bytes < 1024) return bytes + ' B';
		if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
		return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
	}

	function isImage(mime: string): boolean {
		return mime.startsWith('image/');
	}

	async function handleUpload(event: Event) {
		const input = event.target as HTMLInputElement;
		const file = input?.files?.[0];
		if (!file) return;

		uploading = true;
		errorMessage = '';

		try {
			const uploaded = await api.uploadMedia(file);
			media = [uploaded, ...media];
		} catch {
			errorMessage = 'Upload failed. Please try again.';
		} finally {
			uploading = false;
			if (fileInput) fileInput.value = '';
		}
	}

	async function deleteMedia(id: string) {
		deleting = id;
		errorMessage = '';

		try {
			await api.deleteMedia(id);
			media = media.filter((m) => m.id !== id);
			if (previewItem?.id === id) previewItem = null;
		} catch {
			errorMessage = 'Failed to delete media. Please try again.';
		} finally {
			deleting = null;
		}
	}
</script>

<svelte:head>
	<title>Media Library | JapaConnect</title>
	<meta name="description" content="Manage your uploaded media files." />
</svelte:head>

<main class="mx-auto max-w-6xl px-4 py-8">
	<header class="mb-8 flex items-start justify-between">
		<div>
			<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
				Media Library
			</h1>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{media.length} file{media.length !== 1 ? 's' : ''} uploaded
			</p>
		</div>
		<div>
			<input
				bind:this={fileInput}
				type="file"
				accept="image/*,video/*"
				onchange={handleUpload}
				class="hidden"
				id="media-upload"
			/>
			<label
				for="media-upload"
				class="flex cursor-pointer items-center gap-1.5 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
				style="background-color: var(--color-primary)"
			>
				{#if uploading}
					<Loader2 class="h-4 w-4 animate-spin" />
					Uploading...
				{:else}
					<Upload class="h-4 w-4" />
					Upload
				{/if}
			</label>
		</div>
	</header>

	{#if errorMessage}
		<div
			class="mb-4 rounded-lg border px-4 py-3 text-sm"
			style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
		>
			{errorMessage}
		</div>
	{/if}

	{#if media.length === 0}
		<EmptyState
			icon={Image}
			title="No media yet"
			message="Upload images and videos to use in your content."
		/>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4">
			{#each media as item (item.id)}
				<div
					class="group relative overflow-hidden rounded-xl border transition-all hover:shadow-md"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					{#if isImage(item.mime_type)}
						<div class="aspect-square overflow-hidden bg-black/5">
							<img
								src={item.url}
								alt={item.alt_text || item.filename}
								class="h-full w-full object-cover transition-transform group-hover:scale-105"
							/>
						</div>
					{:else}
						<div
							class="flex aspect-square items-center justify-center"
							style="background-color: var(--color-background)"
						>
							<Image class="h-12 w-12" style="color: var(--color-muted)" />
						</div>
					{/if}

					<div class="p-3">
						<p class="truncate text-sm font-medium" style="color: var(--color-foreground)">
							{item.filename}
						</p>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">
							{formatFileSize(item.file_size)}
						</p>
					</div>

					<div
						class="absolute right-2 top-2 flex gap-1 opacity-0 transition-opacity group-hover:opacity-100"
					>
						<button
							onclick={() => (previewItem = item)}
							class="rounded-md p-1.5 backdrop-blur-sm transition-colors"
							style="background-color: color-mix(in srgb, var(--color-surface) 80%, transparent); color: var(--color-foreground)"
							aria-label="Preview"
						>
							<Eye class="h-4 w-4" />
						</button>
						<button
							onclick={() => deleteMedia(item.id)}
							disabled={deleting === item.id}
							class="rounded-md p-1.5 backdrop-blur-sm transition-colors"
							style="background-color: color-mix(in srgb, var(--color-surface) 80%, transparent); color: var(--color-danger)"
							aria-label="Delete"
						>
							{#if deleting === item.id}
								<Loader2 class="h-4 w-4 animate-spin" />
							{:else}
								<Trash2 class="h-4 w-4" />
							{/if}
						</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</main>

<!-- Preview Modal -->
{#if previewItem}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
		onclick={() => (previewItem = null)}
		onkeydown={(e) => e.key === 'Escape' && (previewItem = null)}
	>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="relative max-h-[85vh] max-w-4xl overflow-auto rounded-2xl p-4"
			style="background-color: var(--color-surface)"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
		>
			<button
				onclick={() => (previewItem = null)}
				class="absolute right-3 top-3 rounded-full p-1.5 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
				style="color: var(--color-muted)"
			>
				<X class="h-5 w-5" />
			</button>

			{#if isImage(previewItem.mime_type)}
				<img
					src={previewItem.url}
					alt={previewItem.alt_text || previewItem.filename}
					class="max-h-[70vh] rounded-lg object-contain"
				/>
			{/if}

			<div class="mt-4 space-y-1">
				<p class="text-sm font-medium" style="color: var(--color-foreground)">
					{previewItem.filename}
				</p>
				<p class="text-xs" style="color: var(--color-muted)">
					{previewItem.mime_type} &middot; {formatFileSize(previewItem.file_size)}
				</p>
			</div>
		</div>
	</div>
{/if}
