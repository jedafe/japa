<script lang="ts">
	import {
		ArrowLeft,
		Save,
		Eye,
		Send,
		ChevronDown,
		Loader2,
		type Icon as IconType,
	} from 'lucide-svelte';
	interface Props {
		contentType: { slug: string; label: string; icon: string };
		status?: 'draft' | 'publishing' | 'published';
		onBack: () => void;
		onSaveDraft: () => void;
		onPublish: () => void;
		onPreview: () => void;
		isSaving?: boolean;
		isPublishing?: boolean;
	}

	let {
		contentType,
		status = 'draft',
		onBack,
		onSaveDraft,
		onPublish,
		onPreview,
		isSaving = false,
		isPublishing = false,
	}: Props = $props();
</script>

<header class="sticky top-0 z-50 border-b border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-900">
	<div class="flex h-14 items-center justify-between px-4">
		<!-- Left: Back + Content Type Badge -->
		<div class="flex items-center gap-3">
			<button
				onclick={onBack}
				class="rounded-md p-2 text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-100"
			>
				<ArrowLeft class="h-5 w-5" />
			</button>
			<span
				class="inline-flex items-center gap-1.5 rounded-full border border-blue-200 bg-blue-50 px-2.5 py-0.5 text-xs font-medium text-blue-700 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-300"
			>
				{contentType.label}
			</span>
		</div>

		<!-- Center: Status Indicator -->
		<div class="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
			{#if isSaving}
				<span class="flex items-center gap-1.5">
					<Loader2 class="h-4 w-4 animate-spin" />
					Saving...
				</span>
			{:else if isPublishing}
				<span class="flex items-center gap-1.5">
					<Loader2 class="h-4 w-4 animate-spin" />
					Publishing...
				</span>
			{:else if status === 'published'}
				<span class="inline-flex items-center gap-1.5 text-green-600 dark:text-green-400">
					<span class="h-2 w-2 rounded-full bg-green-500"></span>
					Published
				</span>
			{:else}
				<span class="inline-flex items-center gap-1.5 text-gray-400 dark:text-gray-500">
					<span class="h-2 w-2 rounded-full bg-gray-300 dark:bg-gray-600"></span>
					Draft
				</span>
			{/if}
		</div>

		<!-- Right: Actions -->
		<div class="flex items-center gap-2">
			<button
				onclick={onSaveDraft}
				disabled={isSaving}
				class="inline-flex items-center gap-1.5 rounded-md border border-gray-200 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
			>
				{#if isSaving}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<Save class="h-4 w-4" />
				{/if}
				Save Draft
			</button>

			<button
				onclick={onPreview}
				class="inline-flex items-center gap-1.5 rounded-md border border-gray-200 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
			>
				<Eye class="h-4 w-4" />
				Preview
			</button>

			<button
				onclick={onPublish}
				disabled={isPublishing}
				class="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-1.5 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
			>
				{#if isPublishing}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<Send class="h-4 w-4" />
				{/if}
				Publish
				<ChevronDown class="h-3.5 w-3.5" />
			</button>
		</div>
	</div>
</header>
