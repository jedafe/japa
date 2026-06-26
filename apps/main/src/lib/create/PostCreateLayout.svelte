<script lang="ts">
	import EditorHeader from './EditorHeader.svelte';
	import EditorSidebar from './EditorSidebar.svelte';

	interface Props {
		contentType: { slug: string; label: string; icon: string };
		initialTitle?: string;
		initialContent?: string;
		onSave: (data: { title: string; content: string; metadata: Record<string, unknown> }) => void;
		onPublish: (data: { title: string; content: string; metadata: Record<string, unknown> }) => void;
	}

	let {
		contentType,
		initialTitle = '',
		initialContent = '',
		onSave,
		onPublish,
	}: Props = $props();

	let title = $state(initialTitle);
	let content = $state(initialContent);
	let tags = $state<string[]>([]);
	let featuredImage = $state<string | null>(null);
	let spaceId = $state<string | null>(null);
	let scheduledAt = $state<string | null>(null);
	let visibility = $state<'public' | 'members' | 'private'>('public');
	let seoTitle = $state('');
	let seoDescription = $state('');
	let isSaving = $state(false);
	let isPublishing = $state(false);
	let status = $state<'draft' | 'publishing' | 'published'>('draft');
	let sidebarOpen = $state(true);

	function buildMetadata(): Record<string, unknown> {
		return {
			tags,
			featuredImage,
			spaceId,
			scheduledAt,
			visibility,
			seoTitle,
			seoDescription,
		};
	}

	function handleSave() {
		if (isSaving) return;
		isSaving = true;
		status = 'draft';
		onSave({ title, content, metadata: buildMetadata() });
		setTimeout(() => {
			isSaving = false;
		}, 1000);
	}

	function handlePublish() {
		if (isPublishing) return;
		isPublishing = true;
		status = 'publishing';
		onPublish({ title, content, metadata: buildMetadata() });
		setTimeout(() => {
			isPublishing = false;
			status = 'published';
		}, 1500);
	}

	function handlePreview() {
		// Placeholder — preview opens in a new tab when wired up
	}

	// Autosave every 30 seconds when there is content
	$effect(() => {
		const hasContent = title.length > 0 || content.length > 0;
		if (!hasContent) return;

		const interval = setInterval(() => {
			handleSave();
		}, 30_000);

		return () => clearInterval(interval);
	});

	function handleTagsChange(newTags: string[]) {
		tags = newTags;
	}

	function handleImageChange(url: string) {
		featuredImage = url || null;
	}

	function handleSpaceChange(id: string) {
		spaceId = id || null;
	}

	function handleScheduleChange(date: string | null) {
		scheduledAt = date;
	}

	function handleVisibilityChange(v: string) {
		visibility = v as 'public' | 'members' | 'private';
	}

	function handleSeoChange(seoT: string, seoD: string) {
		seoTitle = seoT;
		seoDescription = seoD;
	}

	function toggleSidebar() {
		sidebarOpen = !sidebarOpen;
	}
</script>

<div class="flex min-h-screen flex-col bg-white dark:bg-gray-950">
	<EditorHeader
		{contentType}
		{status}
		{isSaving}
		{isPublishing}
		onBack={() => history.back()}
		onSaveDraft={handleSave}
		onPublish={handlePublish}
		onPreview={handlePreview}
	/>

	<div class="flex flex-1 overflow-hidden">
		<!-- Main Editor Area -->
		<main class="flex-1 overflow-y-auto">
			<div class="mx-auto max-w-3xl px-4 py-8 sm:px-6 lg:px-8">
				<!-- Title Input -->
				<input
					type="text"
					bind:value={title}
					placeholder="Untitled"
					class="mb-6 w-full border-none bg-transparent text-4xl font-bold tracking-tight text-gray-900 placeholder-gray-300 outline-none dark:text-gray-100 dark:placeholder-gray-600"
				/>

				<!-- TipTap Editor Placeholder -->
				<div class="min-h-[500px]">
					{#if content}
						{@html content}
					{:else}
						<p class="text-lg leading-relaxed text-gray-400 dark:text-gray-500">
							Start writing...
						</p>
					{/if}
				</div>
			</div>
		</main>

		<!-- Sidebar Toggle (visible on mobile) -->
		<button
			onclick={toggleSidebar}
			class="fixed bottom-4 right-4 z-40 rounded-full bg-gray-900 p-3 text-white shadow-lg transition-colors hover:bg-gray-700 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200 lg:hidden"
			aria-label={sidebarOpen ? 'Close sidebar' : 'Open sidebar'}
		>
			<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
				{#if sidebarOpen}
					<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
				{:else}
					<path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h16" />
				{/if}
			</svg>
		</button>

		<!-- Right Sidebar -->
		<div
			class="hidden transition-all duration-200 lg:block"
			class:w-80={sidebarOpen}
			class:w-0={!sidebarOpen}
			class:overflow-hidden={!sidebarOpen}
		>
			{#if sidebarOpen}
				<EditorSidebar
					{tags}
					{featuredImage}
					{spaceId}
					{scheduledAt}
					{visibility}
					{seoTitle}
					{seoDescription}
					onTagsChange={handleTagsChange}
					onImageChange={handleImageChange}
					onSpaceChange={handleSpaceChange}
					onScheduleChange={handleScheduleChange}
					onVisibilityChange={handleVisibilityChange}
					onSeoChange={handleSeoChange}
				/>
			{/if}
		</div>

		<!-- Mobile Sidebar Overlay -->
		{#if sidebarOpen}
			<div
				class="fixed inset-0 z-30 bg-black/20 backdrop-blur-sm lg:hidden"
				role="button"
				tabindex="-1"
				onclick={toggleSidebar}
				onkeydown={(e) => e.key === 'Escape' && toggleSidebar()}
			></div>
			<div class="fixed bottom-0 right-0 top-0 z-30 w-80 lg:hidden">
				<EditorSidebar
					{tags}
					{featuredImage}
					{spaceId}
					{scheduledAt}
					{visibility}
					{seoTitle}
					{seoDescription}
					onTagsChange={handleTagsChange}
					onImageChange={handleImageChange}
					onSpaceChange={handleSpaceChange}
					onScheduleChange={handleScheduleChange}
					onVisibilityChange={handleVisibilityChange}
					onSeoChange={handleSeoChange}
				/>
			</div>
		{/if}
	</div>
</div>
