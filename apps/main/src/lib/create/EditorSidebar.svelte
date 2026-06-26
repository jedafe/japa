<script lang="ts">
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
	} from 'lucide-svelte';

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

	const spaces = [
		{ id: 'general', label: 'General Discussion' },
		{ id: 'visa', label: 'Visa & Immigration' },
		{ id: 'housing', label: 'Housing & Accommodation' },
		{ id: 'jobs', label: 'Job Opportunities' },
		{ id: 'culture', label: 'Culture & Lifestyle' },
		{ id: 'healthcare', label: 'Healthcare' },
		{ id: 'education', label: 'Education' },
		{ id: 'finance', label: 'Finance & Banking' },
	];

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
							<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">No image selected</p>
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
					<select
						value={spaceId || ''}
						onchange={handleSpaceChange}
						class="w-full appearance-none rounded-lg border border-gray-300 bg-white px-3 py-2 pr-10 text-sm text-gray-900 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100"
					>
						<option value="">No space</option>
						{#each spaces as space}
							<option value={space.id}>{space.label}</option>
						{/each}
					</select>
					<ChevronDown class="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-500 dark:text-gray-400" />
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
								<span class="text-sm font-medium text-gray-900 dark:text-gray-100">Public</span>
							</div>
							<p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">Visible to everyone</p>
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
								<span class="text-sm font-medium text-gray-900 dark:text-gray-100">Members Only</span>
							</div>
							<p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">Visible to registered members</p>
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
								<span class="text-sm font-medium text-gray-900 dark:text-gray-100">Private</span>
							</div>
							<p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">Only visible to you</p>
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
										<option value="">Select...</option>
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
									<span class="text-sm text-gray-700 dark:text-gray-300">Enable</span>
								</label>
							{/if}
						</div>
					{/each}
				</div>
			</details>
		{/if}
	</div>
</aside>
