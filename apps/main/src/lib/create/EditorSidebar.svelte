<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import {
		X,
		Upload,
		Users,
		Lock,
		Globe,
		Tag,
		Image,
		MapPin,
		Clock,
		Search,
		ChevronDown,
		ClipboardList,
		Link,
		ExternalLink,
		Loader2,
	} from 'lucide-svelte';
	import LinkPreview from '$lib/components/editor/LinkPreview.svelte';
	import { api } from '$lib/api';
	import { onMount } from 'svelte';

	interface CustomField {
		key: string;
		label: string;
		type: string;
		value: unknown;
		options?: string[];
	}

	interface Props {
		tags?: string[];
		featuredImage?: string | null;
		spaceId?: string | null;
		scheduledAt?: string | null;
		visibility?: 'public' | 'members' | 'private';
		seoTitle?: string;
		seoDescription?: string;
		onTagsChange?: (tags: string[]) => void;
		onImageChange?: (url: string) => void;
		onSpaceChange?: (spaceId: string) => void;
		onScheduleChange?: (date: string | null) => void;
		onVisibilityChange?: (visibility: string) => void;
		onSeoChange?: (title: string, description: string) => void;
		customFields?: CustomField[];
		onCustomFieldChange?: (key: string, value: unknown) => void;
	}

	let {
		tags = [],
		featuredImage = null,
		spaceId = null,
		scheduledAt = null,
		visibility = 'public',
		seoTitle = '',
		seoDescription = '',
		onTagsChange,
		onImageChange,
		onSpaceChange,
		onScheduleChange,
		onVisibilityChange,
		onSeoChange,
		customFields = [],
		onCustomFieldChange,
	}: Props = $props();

	let tagInput = $state('');
	let previewUrl = $state('');
	let previewUrlInput = $state('');
	let showPreview = $state(false);
	let spaces = $state<{ id: string; name: string }[]>([]);
	let spacesLoading = $state(true);

	onMount(async () => {
		try {
			const result: any = await api.listSpaces({ page_size: 100 });
			if (Array.isArray(result)) {
				spaces = result.map((s: any) => ({ id: s.id || s.slug, name: s.name }));
			} else if (result?.data) {
				spaces = (result.data as any[]).map((s) => ({ id: s.id || s.slug, name: s.name }));
			}
		} catch {
			spaces = [];
		} finally {
			spacesLoading = false;
		}
	});

	function checkLinkPreview() {
		const trimmed = previewUrlInput.trim();
		if (!trimmed) return;
		// Basic URL validation
		try {
			new URL(trimmed);
			previewUrl = trimmed;
			showPreview = true;
		} catch {
			// Try adding https:// if missing
			try {
				new URL('https://' + trimmed);
				previewUrl = 'https://' + trimmed;
				previewUrlInput = previewUrl;
				showPreview = true;
			} catch {
				// Invalid URL
			}
		}
	}

	function clearLinkPreview() {
		previewUrl = '';
		previewUrlInput = '';
		showPreview = false;
	}

	function handlePreviewKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			checkLinkPreview();
		}
	}

	function addTags() {
		const newTags = tagInput
			.split(',')
			.map((t) => t.trim())
			.filter((t) => t && !tags.includes(t));
		if (newTags.length > 0) {
			onTagsChange?.([...tags, ...newTags]);
			tagInput = '';
		}
	}

	function removeTag(tag: string) {
		onTagsChange?.(tags.filter((t) => t !== tag));
	}

	function handleTagKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			addTags();
		}
	}

	function handleImageInput(e: Event) {
		const input = e.target as HTMLInputElement;
		if (input.value) {
			onImageChange?.(input.value);
		}
	}

	function handleSpaceChange(e: Event) {
		const select = e.target as HTMLSelectElement;
		onSpaceChange?.(select.value);
	}

	function handleScheduleChange(e: Event) {
		const input = e.target as HTMLInputElement;
		onScheduleChange?.(input.value || null);
	}

	function handleVisibilityChange(value: string) {
		onVisibilityChange?.(value);
	}

	function handleSeoTitleChange(e: Event) {
		const input = e.target as HTMLInputElement;
		onSeoChange?.(input.value, seoDescription);
	}

	function handleSeoDescriptionChange(e: Event) {
		const textarea = e.target as HTMLTextAreaElement;
		onSeoChange?.(seoTitle, textarea.value);
	}

	function handleCustomFieldChange(key: string, value: unknown) {
		onCustomFieldChange?.(key, value);
	}
</script>

<aside class="flex h-full w-80 flex-col border-l border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-900">
	<div class="flex-1 overflow-y-auto">
		<!-- Tags Section -->
		<details open class="border-b border-gray-200 dark:border-gray-700">
			<summary class="flex cursor-pointer items-center gap-2 p-4 text-sm font-medium text-gray-900 dark:text-gray-100">
				<Tag class="h-4 w-4 text-gray-500 dark:text-gray-400" />
				Tags
			</summary>
			<div class="px-4 pb-4">
				<div class="flex gap-2">
					<input
						type="text"
						bind:value={tagInput}
						onkeydown={handleTagKeydown}
						placeholder="Add tags (comma separated)"
						class="flex-1 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-500 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-400"
					/>
					<button
						onclick={addTags}
						class="rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
					>
						Add
					</button>
				</div>
				{#if tags.length > 0}
					<div class="mt-3 flex flex-wrap gap-2">
						{#each tags as tag}
							<span class="inline-flex items-center gap-1 rounded-full border border-gray-200 bg-white px-2.5 py-1 text-xs font-medium text-gray-700 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300">
								{tag}
								<button
									onclick={() => removeTag(tag)}
									class="ml-0.5 rounded-full p-0.5 text-gray-400 hover:bg-gray-200 hover:text-gray-600 dark:hover:bg-gray-700 dark:hover:text-gray-300"
								>
									<X class="h-3 w-3" />
								</button>
							</span>
						{/each}
					</div>
				{/if}
			</div>
		</details>

		<!-- Featured Image Section -->
		<details class="border-b border-gray-200 dark:border-gray-700">
			<summary class="flex cursor-pointer items-center gap-2 p-4 text-sm font-medium text-gray-900 dark:text-gray-100">
				<Image class="h-4 w-4 text-gray-500 dark:text-gray-400" />
				Featured Image
			</summary>
			<div class="px-4 pb-4">
				<input
					type="url"
					value={featuredImage || ''}
					oninput={handleImageInput}
					placeholder="Enter image URL"
					class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-500 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-400"
				/>
				{#if featuredImage}
					<div class="relative mt-3 overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
						<img
							src={featuredImage}
							alt="Featured"
							class="h-32 w-full object-cover"
						/>
						<button
							onclick={() => onImageChange?.('')}
							class="absolute top-2 right-2 rounded-full bg-black/50 p-1 text-white hover:bg-black/70"
						>
							<X class="h-4 w-4" />
						</button>
					</div>
				{:else}
					<div class="mt-3 flex h-32 items-center justify-center rounded-lg border-2 border-dashed border-gray-300 bg-gray-100 dark:border-gray-600 dark:bg-gray-800">
						<div class="text-center">
							<Upload class="mx-auto h-8 w-8 text-gray-400 dark:text-gray-500" />
							<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{t('lib.create.EditorSidebar.svelte.no_image_selected')}</p>
						</div>
					</div>
				{/if}
			</div>
		</details>

		<!-- Space Section -->
		<details class="border-b border-gray-200 dark:border-gray-700">
			<summary class="flex cursor-pointer items-center gap-2 p-4 text-sm font-medium text-gray-900 dark:text-gray-100">
				<MapPin class="h-4 w-4 text-gray-500 dark:text-gray-400" />
				Space
			</summary>
			<div class="px-4 pb-4">
				<div class="relative">
					{#if spacesLoading}
						<div class="flex items-center gap-2 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-500 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-400">
							<Loader2 class="h-3.5 w-3.5 animate-spin" />
							Loading spaces...
						</div>
					{:else}
						<select
							value={spaceId || ''}
							onchange={handleSpaceChange}
							class="w-full appearance-none rounded-lg border border-gray-300 bg-white px-3 py-2 pr-10 text-sm text-gray-900 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100"
						>
							<option value="">{t('lib.create.EditorSidebar.svelte.no_space')}</option>
							{#each spaces as space}
								<option value={space.id}>{space.name}</option>
							{/each}
						</select>
						<ChevronDown class="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-500 dark:text-gray-400" />
					{/if}
				</div>
			</div>
		</details>

		<!-- Schedule Section -->
		<details class="border-b border-gray-200 dark:border-gray-700">
			<summary class="flex cursor-pointer items-center gap-2 p-4 text-sm font-medium text-gray-900 dark:text-gray-100">
				<Clock class="h-4 w-4 text-gray-500 dark:text-gray-400" />
				Schedule
			</summary>
			<div class="px-4 pb-4">
				<input
					type="datetime-local"
					value={scheduledAt || ''}
					onchange={handleScheduleChange}
					class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100"
				/>
				<p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
					{#if scheduledAt}
						Will be published at the scheduled time
					{:else}
						Leave empty to publish immediately
					{/if}
				</p>
			</div>
		</details>

		<!-- Visibility Section -->
		<details open class="border-b border-gray-200 dark:border-gray-700">
			<summary class="flex cursor-pointer items-center gap-2 p-4 text-sm font-medium text-gray-900 dark:text-gray-100">
				{#if visibility === 'public'}
					<Globe class="h-4 w-4 text-gray-500 dark:text-gray-400" />
				{:else if visibility === 'members'}
					<Users class="h-4 w-4 text-gray-500 dark:text-gray-400" />
				{:else}
					<Lock class="h-4 w-4 text-gray-500 dark:text-gray-400" />
				{/if}
				Visibility
			</summary>
			<div class="px-4 pb-4">
				<div class="space-y-2">
					<label class="flex cursor-pointer items-center gap-3 rounded-lg border border-gray-200 bg-white p-3 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:hover:bg-gray-750">
						<input
							type="radio"
							name="visibility"
							value="public"
							checked={visibility === 'public'}
							onchange={() => handleVisibilityChange('public')}
							class="h-4 w-4 border-gray-300 text-blue-600 focus:ring-blue-500 dark:border-gray-600"
						/>
						<div class="flex-1">
							<div class="flex items-center gap-2">
								<Globe class="h-4 w-4 text-gray-600 dark:text-gray-400" />
								<span class="text-sm font-medium text-gray-900 dark:text-gray-100">{t('lib.create.EditorSidebar.svelte.public')}</span>
							</div>
							<p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{t('lib.create.EditorSidebar.svelte.visible_to_everyone')}</p>
						</div>
					</label>

					<label class="flex cursor-pointer items-center gap-3 rounded-lg border border-gray-200 bg-white p-3 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:hover:bg-gray-750">
						<input
							type="radio"
							name="visibility"
							value="members"
							checked={visibility === 'members'}
							onchange={() => handleVisibilityChange('members')}
							class="h-4 w-4 border-gray-300 text-blue-600 focus:ring-blue-500 dark:border-gray-600"
						/>
						<div class="flex-1">
							<div class="flex items-center gap-2">
								<Users class="h-4 w-4 text-gray-600 dark:text-gray-400" />
								<span class="text-sm font-medium text-gray-900 dark:text-gray-100">{t('lib.create.EditorSidebar.svelte.members_only')}</span>
							</div>
							<p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{t('lib.create.EditorSidebar.svelte.visible_to_registered_members')}</p>
						</div>
					</label>

					<label class="flex cursor-pointer items-center gap-3 rounded-lg border border-gray-200 bg-white p-3 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:hover:bg-gray-750">
						<input
							type="radio"
							name="visibility"
							value="private"
							checked={visibility === 'private'}
							onchange={() => handleVisibilityChange('private')}
							class="h-4 w-4 border-gray-300 text-blue-600 focus:ring-blue-500 dark:border-gray-600"
						/>
						<div class="flex-1">
							<div class="flex items-center gap-2">
								<Lock class="h-4 w-4 text-gray-600 dark:text-gray-400" />
								<span class="text-sm font-medium text-gray-900 dark:text-gray-100">{t('lib.create.EditorSidebar.svelte.private')}</span>
							</div>
							<p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{t('lib.create.EditorSidebar.svelte.only_visible_to_you')}</p>
						</div>
					</label>
				</div>
			</div>
		</details>

		<!-- SEO Section -->
		<details class="border-b border-gray-200 dark:border-gray-700">
			<summary class="flex cursor-pointer items-center gap-2 p-4 text-sm font-medium text-gray-900 dark:text-gray-100">
				<Search class="h-4 w-4 text-gray-500 dark:text-gray-400" />
				SEO
			</summary>
			<div class="space-y-3 px-4 pb-4">
				<div>
					<label for="seo-title" class="mb-1 block text-xs font-medium text-gray-700 dark:text-gray-300">
						Title
					</label>
					<input
						id="seo-title"
						type="text"
						value={seoTitle}
						oninput={handleSeoTitleChange}
						placeholder="SEO title"
						class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-500 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-400"
					/>
					<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
						{seoTitle.length}/60 characters
					</p>
				</div>
				<div>
					<label for="seo-description" class="mb-1 block text-xs font-medium text-gray-700 dark:text-gray-300">
						Description
					</label>
					<textarea
						id="seo-description"
						value={seoDescription}
						oninput={handleSeoDescriptionChange}
						placeholder="SEO description"
						rows="3"
						class="w-full resize-none rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-500 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-400"
					></textarea>
					<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
						{seoDescription.length}/160 characters
					</p>
				</div>
			</div>
		</details>

		<!-- Link Preview Checker Section -->
		<details class="border-b border-gray-200 dark:border-gray-700">
			<summary class="flex cursor-pointer items-center gap-2 p-4 text-sm font-medium text-gray-900 dark:text-gray-100">
				<Link class="h-4 w-4 text-gray-500 dark:text-gray-400" />
				Link Preview
			</summary>
			<div class="px-4 pb-4">
				<p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
					Check how a link will look when shared on JapaConnect
				</p>
				<div class="flex gap-2">
					<input
						type="url"
						bind:value={previewUrlInput}
						onkeydown={handlePreviewKeydown}
						placeholder="Paste a URL..."
						class="flex-1 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-500 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-400"
					/>
					<button
						onclick={checkLinkPreview}
						class="flex items-center gap-1.5 rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
					>
						<ExternalLink class="h-3.5 w-3.5" />
						Check
					</button>
				</div>

				{#if showPreview && previewUrl}
					<div class="mt-4">
						<div class="mb-2 flex items-center justify-between">
							<span class="text-xs font-medium text-gray-700 dark:text-gray-300">{t('lib.create.EditorSidebar.svelte.preview')}</span>
							<button
								onclick={clearLinkPreview}
								class="rounded p-1 text-gray-400 transition-colors hover:bg-gray-200 hover:text-gray-600 dark:hover:bg-gray-700 dark:hover:text-gray-300"
								aria-label="Clear preview"
							>
								<X class="h-3.5 w-3.5" />
							</button>
						</div>
						<div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
							<LinkPreview url={previewUrl} />
						</div>
					</div>
				{:else if showPreview}
					<div class="mt-4 flex h-24 items-center justify-center rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 dark:border-gray-600 dark:bg-gray-800/50">
						<p class="text-xs text-gray-400 dark:text-gray-500">{t('lib.create.EditorSidebar.svelte.enter_a_url_to_preview')}</p>
					</div>
				{/if}
			</div>
		</details>

		<!-- Custom Fields Section -->
		{#if customFields.length > 0}
			<details class="border-b border-gray-200 dark:border-gray-700">
				<summary class="flex cursor-pointer items-center gap-2 p-4 text-sm font-medium text-gray-900 dark:text-gray-100">
					<ClipboardList class="h-4 w-4 text-gray-500 dark:text-gray-400" />
					Custom Fields
				</summary>
				<div class="space-y-3 px-4 pb-4">
					{#each customFields as field}
						<div>
							<label for="field-{field.key}" class="mb-1 block text-xs font-medium text-gray-700 dark:text-gray-300">
								{field.label}
							</label>
							{#if field.type === 'text'}
								<input
									id="field-{field.key}"
									type="text"
									value={String(field.value || '')}
									oninput={(e) => handleCustomFieldChange(field.key, (e.target as HTMLInputElement).value)}
									class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-500 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-400"
								/>
							{:else if field.type === 'textarea'}
								<textarea
									id="field-{field.key}"
									value={String(field.value || '')}
									oninput={(e) => handleCustomFieldChange(field.key, (e.target as HTMLTextAreaElement).value)}
									rows="3"
									class="w-full resize-none rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-500 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-400"
								></textarea>
							{:else if field.type === 'select' && field.options}
								<div class="relative">
									<select
										id="field-{field.key}"
										value={String(field.value || '')}
										onchange={(e) => handleCustomFieldChange(field.key, (e.target as HTMLSelectElement).value)}
										class="w-full appearance-none rounded-lg border border-gray-300 bg-white px-3 py-2 pr-10 text-sm text-gray-900 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100"
									>
										<option value="">{t('lib.create.EditorSidebar.svelte.select')}</option>
										{#each field.options as option}
											<option value={option}>{option}</option>
										{/each}
									</select>
									<ChevronDown class="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-500 dark:text-gray-400" />
								</div>
							{:else if field.type === 'number'}
								<input
									id="field-{field.key}"
									type="number"
									value={field.value ?? ''}
									oninput={(e) => handleCustomFieldChange(field.key, Number((e.target as HTMLInputElement).value))}
									class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-500 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-400"
								/>
							{:else if field.type === 'date'}
								<input
									id="field-{field.key}"
									type="date"
									value={String(field.value || '')}
									oninput={(e) => handleCustomFieldChange(field.key, (e.target as HTMLInputElement).value)}
									class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-500 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-400"
								/>
							{:else if field.type === 'checkbox'}
								<label class="flex items-center gap-2">
									<input
										id="field-{field.key}"
										type="checkbox"
										checked={Boolean(field.value)}
										onchange={(e) => handleCustomFieldChange(field.key, (e.target as HTMLInputElement).checked)}
										class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500 dark:border-gray-600"
									/>
									<span class="text-sm text-gray-700 dark:text-gray-300">{t('lib.create.EditorSidebar.svelte.enable')}</span>
								</label>
							{/if}
						</div>
					{/each}
				</div>
			</details>
		{/if}
	</div>
</aside>
