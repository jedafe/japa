<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { X, Check } from 'lucide-svelte';
	import {
		Layout,
		Type,
		Image,
		MousePointerClick,
		BarChart3,
		HelpCircle,
		Quote,
		GitCompare,
		FileText,
		Minus,
		List,
	} from 'lucide-svelte';
	import { getBlockTypeByType } from './registry';
	import type { BlockInstance } from './types';

	interface Props {
		block: BlockInstance | null;
		onApply: (blockId: string, updatedProps: Record<string, unknown>) => void;
		onCancel: () => void;
	}

	let { block, onApply, onCancel }: Props = $props();

	type FieldType = 'text' | 'textarea' | 'number' | 'select' | 'boolean' | 'color' | 'url';

	interface FieldDef {
		key: string;
		label: string;
		type: FieldType;
		options?: string[];
	}

	const ICON_MAP: Record<string, typeof Layout> = {
		layout: Layout,
		type: Type,
		image: Image,
		'mouse-pointer-click': MousePointerClick,
		'bar-chart-3': BarChart3,
		'help-circle': HelpCircle,
		quote: Quote,
		'git-compare': GitCompare,
		'file-text': FileText,
		minus: Minus,
		list: List,
	};

	/** Known select options for fields that should render as dropdowns. */
	const SELECT_OPTIONS: Record<string, string[]> = {
		layout: ['single', 'horizontal', 'vertical', 'grid'],
		style: ['primary', 'secondary', 'minimal', 'space', 'divider'],
		variant: ['filled', 'outlined', 'ghost'],
		display_mode: ['table', 'cards', 'inline'],
		sort_order: ['asc', 'desc'],
		sort_by: ['created_at', 'updated_at', 'title', 'views'],
	};

	/** Keys that should always render as textarea. */
	const TEXTAREA_KEYS = new Set(['content', 'quote']);

	/** Keys that should render as URL input. */
	const URL_SUFFIXES = ['_url', '_image', 'background_image'];

	let draftProps = $state<Record<string, unknown>>({});

	$effect(() => {
		if (block) {
			draftProps = structuredClone(block.props);
		} else {
			draftProps = {};
		}
	});

	let blockDef = $derived(block ? getBlockTypeByType(block.block_type) : undefined);

	let BlockIcon = $derived.by(() => {
		if (!blockDef) return Layout;
		return ICON_MAP[blockDef.icon] ?? Layout;
	});

	let fields = $derived.by((): FieldDef[] => {
		if (!blockDef) return [];

		return Object.entries(blockDef.default_props).map(([key, defaultValue]) => ({
			key,
			label: humanize(key),
			type: inferFieldType(key, defaultValue),
			options: SELECT_OPTIONS[key],
		}));
	});

	function inferFieldType(key: string, defaultValue: unknown): FieldType {
		if (typeof defaultValue === 'boolean') return 'boolean';
		if (typeof defaultValue === 'number') return 'number';
		if (typeof defaultValue === 'object' && defaultValue !== null) return 'text';
		if (TEXTAREA_KEYS.has(key)) return 'textarea';
		if (URL_SUFFIXES.some((suffix) => key.endsWith(suffix))) return 'url';
		if (SELECT_OPTIONS[key]) return 'select';
		return 'text';
	}

	function humanize(key: string): string {
		return key
			.replace(/_/g, ' ')
			.replace(/\b\w/g, (c) => c.toUpperCase());
	}

	function handleApply() {
		if (!block) return;
		onApply(block.block_id, { ...draftProps });
	}

	function getIconComponent(iconName: string): typeof Layout {
		return ICON_MAP[iconName] ?? Layout;
	}
</script>

{#if block}
	<div
		class="flex h-full flex-col border-l border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-900"
	>
		<!-- Header -->
		<div class="flex items-center gap-3 border-b border-gray-100 px-4 py-3 dark:border-gray-800">
			<span
				class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400"
			>
				<BlockIcon class="h-4 w-4" />
			</span>
			<div class="min-w-0 flex-1">
				<h3 class="truncate text-sm font-semibold text-gray-900 dark:text-gray-100">
					{blockDef?.name ?? block.block_type}
				</h3>
				<p class="truncate text-xs text-gray-500 dark:text-gray-400">
					{blockDef?.description ?? ''}
				</p>
			</div>
			<button
				type="button"
				class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300"
				aria-label="Close settings panel"
				onclick={onCancel}
			>
				<X class="h-4 w-4" />
			</button>
		</div>

		<!-- Form fields -->
		<div class="flex-1 overflow-y-auto px-4 py-4">
			{#if fields.length === 0}
				<p class="py-8 text-center text-sm text-gray-400 dark:text-gray-500">
					This block has no editable properties.
				</p>
			{:else}
				<div class="flex flex-col gap-4">
					{#each fields as field (field.key)}
						<div class="flex flex-col gap-1.5">
							<label
								for="field-{field.key}"
								class="text-xs font-medium text-gray-600 dark:text-gray-400"
							>
								{field.label}
							</label>

							{#if field.type === 'text'}
								<input
									id="field-{field.key}"
									type="text"
									value={draftProps[field.key] ?? ''}
									oninput={(e) => {
										draftProps[field.key] = (e.currentTarget as HTMLInputElement).value;
									}}
									class="rounded-md border border-gray-200 bg-gray-50 px-3 py-1.5 text-sm text-gray-900 outline-none transition-colors focus:border-blue-400 focus:bg-white focus:ring-2 focus:ring-blue-400/20 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100 dark:focus:border-blue-500"
								/>

							{:else if field.type === 'textarea'}
								<textarea
									id="field-{field.key}"
									rows="4"
									value={String(draftProps[field.key] ?? '')}
									oninput={(e) => {
										draftProps[field.key] = (e.currentTarget as HTMLTextAreaElement).value;
									}}
									class="resize-y rounded-md border border-gray-200 bg-gray-50 px-3 py-1.5 text-sm text-gray-900 outline-none transition-colors focus:border-blue-400 focus:bg-white focus:ring-2 focus:ring-blue-400/20 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100 dark:focus:border-blue-500"
								></textarea>

							{:else if field.type === 'number'}
								<input
									id="field-{field.key}"
									type="number"
									value={draftProps[field.key] ?? 0}
									oninput={(e) => {
										draftProps[field.key] = Number(
											(e.currentTarget as HTMLInputElement).value
										);
									}}
									class="rounded-md border border-gray-200 bg-gray-50 px-3 py-1.5 text-sm text-gray-900 outline-none transition-colors focus:border-blue-400 focus:bg-white focus:ring-2 focus:ring-blue-400/20 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100 dark:focus:border-blue-500"
								/>

							{:else if field.type === 'select'}
								<select
									id="field-{field.key}"
									value={draftProps[field.key] ?? ''}
									onchange={(e) => {
										draftProps[field.key] = (e.currentTarget as HTMLSelectElement).value;
									}}
									class="rounded-md border border-gray-200 bg-gray-50 px-3 py-1.5 text-sm text-gray-900 outline-none transition-colors focus:border-blue-400 focus:bg-white focus:ring-2 focus:ring-blue-400/20 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100 dark:focus:border-blue-500"
								>
									{#each field.options ?? [] as opt}
										<option value={opt}>{humanize(opt)}</option>
									{/each}
								</select>

							{:else if field.type === 'boolean'}
								{@const isOn = !!draftProps[field.key]}
								<button
									id="field-{field.key}"
									type="button"
									role="switch"
									aria-checked={isOn}
									aria-label="Toggle {field.label}"
									onclick={() => {
										draftProps[field.key] = !draftProps[field.key];
									}}
									class="relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full transition-colors"
									class:bg-blue-500={isOn}
									class:bg-gray-300={!isOn}
								>
									<span
										class="inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform"
										class:translate-x-4={isOn}
										class:translate-x-0.5={!isOn}
									></span>
								</button>

							{:else if field.type === 'color'}
								<div class="flex items-center gap-2">
									<input
										id="field-{field.key}"
										type="color"
										value={draftProps[field.key] ?? '#000000'}
										oninput={(e) => {
											draftProps[field.key] = (e.currentTarget as HTMLInputElement).value;
										}}
										class="h-8 w-8 cursor-pointer rounded border border-gray-200 dark:border-gray-700"
									/>
									<input
										type="text"
										value={draftProps[field.key] ?? ''}
										oninput={(e) => {
											draftProps[field.key] = (e.currentTarget as HTMLInputElement).value;
										}}
										class="flex-1 rounded-md border border-gray-200 bg-gray-50 px-3 py-1.5 text-sm text-gray-900 outline-none transition-colors focus:border-blue-400 focus:bg-white focus:ring-2 focus:ring-blue-400/20 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100 dark:focus:border-blue-500"
										placeholder="#000000"
									/>
								</div>

							{:else if field.type === 'url'}
								<input
									id="field-{field.key}"
									type="url"
									value={draftProps[field.key] ?? ''}
									oninput={(e) => {
										draftProps[field.key] = (e.currentTarget as HTMLInputElement).value;
									}}
									placeholder="https://"
									class="rounded-md border border-gray-200 bg-gray-50 px-3 py-1.5 text-sm text-gray-900 outline-none transition-colors focus:border-blue-400 focus:bg-white focus:ring-2 focus:ring-blue-400/20 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100 dark:focus:border-blue-500"
								/>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Footer actions -->
		<div class="flex items-center justify-end gap-2 border-t border-gray-100 px-4 py-3 dark:border-gray-800">
			<button
				type="button"
				class="rounded-md px-3 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
				onclick={onCancel}
			>
				Cancel
			</button>
			<button
				type="button"
				class="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
				onclick={handleApply}
			>
				<Check class="h-3.5 w-3.5" />
				Apply
			</button>
		</div>
	</div>
{/if}
