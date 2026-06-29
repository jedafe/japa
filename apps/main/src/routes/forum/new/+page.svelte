<script lang="ts">
	import { goto } from '$app/navigation';
	import { ArrowLeft, Loader2, Send } from 'lucide-svelte';
	import { api, getToken } from '$lib/api';
	import { onMount } from 'svelte';

	let title = $state('');
	let body = $state('');
	let selectedSpaceId = $state('');
	let tagInput = $state('');
	let tags = $state<string[]>([]);
	let anonymous = $state(false);
	let loading = $state(false);
	let error = $state('');
	let spaces = $state<Array<{ id: string; name: string; slug: string }>>([]);

	function addTag() {
		const t = tagInput.trim().toLowerCase();
		if (t && !tags.includes(t) && tags.length < 5) {
			tags = [...tags, t];
			tagInput = '';
		}
	}

	function removeTag(tag: string) {
		tags = tags.filter((t) => t !== tag);
	}

	function handleTagKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			addTag();
		}
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';

		if (!title.trim()) {
			error = 'Title is required.';
			return;
		}
		if (!body.trim()) {
			error = 'Body is required.';
			return;
		}

		loading = true;
		try {
			const thread = await api.createForumThread({
				title: title.trim(),
				body: body.trim(),
				space_id: selectedSpaceId || undefined,
				tags: tags.length > 0 ? tags : undefined,
				anonymous
			});
			goto(`/forum/${thread.slug}`);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to create thread. Please try again.';
		} finally {
			loading = false;
		}
	}

	onMount(async () => {
		try {
			const result = await api.listSpaces({ limit: 50 });
			spaces = result;
		} catch {
			spaces = [];
		}
	});
</script>

<svelte:head>
	<title>New Thread | Forum | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-8">
	<header class="mb-8">
		<a
			href="/forum"
			class="mb-4 inline-flex items-center gap-1 text-sm transition-colors hover:underline"
			style="color: var(--color-muted)"
		>
			<ArrowLeft class="h-4 w-4" />
			Back to Forum
		</a>
		<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
			New Thread
		</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			Ask a question or start a discussion.
		</p>
	</header>

	{#if error}
		<div
			class="mb-6 rounded-lg border px-4 py-3 text-sm"
			style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
		>
			{error}
		</div>
	{/if}

	<form onsubmit={handleSubmit} class="space-y-6">
		<!-- Title -->
		<div>
			<label
				for="thread-title"
				class="mb-1.5 block text-sm font-medium"
				style="color: var(--color-foreground)"
			>
				Title
			</label>
			<input
				id="thread-title"
				type="text"
				placeholder="What's your question or topic?"
				bind:value={title}
			 maxlength="200"
				class="w-full rounded-lg border px-3 py-2.5 text-sm outline-none transition-colors focus:ring-2"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
			/>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">{title.length}/200</p>
		</div>

		<!-- Body -->
		<div>
			<label
				for="thread-body"
				class="mb-1.5 block text-sm font-medium"
				style="color: var(--color-foreground)"
			>
				Body
			</label>
			<textarea
				id="thread-body"
				placeholder="Describe your question or topic in detail..."
				bind:value={body}
				rows="8"
				class="w-full resize-y rounded-lg border px-3 py-2.5 text-sm outline-none transition-colors focus:ring-2"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
			></textarea>
		</div>

		<!-- Space selector -->
		<div>
			<label
				for="thread-space"
				class="mb-1.5 block text-sm font-medium"
				style="color: var(--color-foreground)"
			>
				Space <span class="font-normal" style="color: var(--color-muted)">(optional)</span>
			</label>
			<select
				id="thread-space"
				bind:value={selectedSpaceId}
				class="w-full rounded-lg border px-3 py-2.5 text-sm outline-none transition-colors"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
			>
				<option value="">No space</option>
				{#each spaces as space}
					<option value={space.id}>{space.name}</option>
				{/each}
			</select>
		</div>

		<!-- Tags -->
		<div>
			<label
				for="thread-tags"
				class="mb-1.5 block text-sm font-medium"
				style="color: var(--color-foreground)"
			>
				Tags <span class="font-normal" style="color: var(--color-muted)">(up to 5)</span>
			</label>
			<div class="flex gap-2">
				<input
					id="thread-tags"
					type="text"
					placeholder="Add a tag"
					bind:value={tagInput}
					onkeydown={handleTagKeydown}
					maxlength="30"
					class="flex-1 rounded-lg border px-3 py-2 text-sm outline-none transition-colors"
					style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				/>
				<button
					type="button"
					onclick={addTag}
					disabled={!tagInput.trim() || tags.length >= 5}
					class="shrink-0 rounded-lg px-3 py-2 text-sm font-medium transition-colors disabled:opacity-40"
					style="background-color: var(--color-surface); color: var(--color-foreground); border: 1px solid var(--color-border)"
				>
					Add
				</button>
			</div>
			{#if tags.length > 0}
				<div class="mt-2 flex flex-wrap gap-1.5">
					{#each tags as tag}
						<span
							class="inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium"
							style="background-color: var(--color-primary); color: white"
						>
							{tag}
							<button
								type="button"
								onclick={() => removeTag(tag)}
								class="ml-0.5 rounded-full hover:bg-white/20"
								aria-label="Remove tag {tag}"
							>
								&times;
							</button>
						</span>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Anonymous toggle -->
		<label
			class="flex cursor-pointer items-center gap-3 rounded-lg px-4 py-3 transition-colors"
			style="background-color: var(--color-surface); border: 1px solid var(--color-border)"
		>
			<input type="checkbox" bind:checked={anonymous} class="h-4 w-4 rounded" />
			<div>
				<span class="text-sm font-medium" style="color: var(--color-foreground)">
					Post anonymously
				</span>
				<p class="text-xs" style="color: var(--color-muted)">
					Your identity will be hidden from other users.
				</p>
			</div>
		</label>

		<!-- Submit -->
		<button
			type="submit"
			disabled={loading || !title.trim() || !body.trim()}
			class="flex w-full items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
			style="background-color: var(--color-primary)"
		>
			{#if loading}
				<Loader2 class="h-4 w-4 animate-spin" />
				Posting...
			{:else}
				<Send class="h-4 w-4" />
				Post Thread
			{/if}
		</button>
	</form>
</main>
