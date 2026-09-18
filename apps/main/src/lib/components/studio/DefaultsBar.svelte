<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §6 — project-level
	// defaults persisted to localStorage (japa-studio:web-settings).
	import { t } from '$lib/i18n';
	import type { ContentTypeMeta, MeSpace } from '$lib/types';
	import type { StudioWebDefaults } from '$lib/studio/defaults';
	import { saveStudioDefaults } from '$lib/studio/defaults';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import { ChevronDown, ChevronUp, Save } from 'lucide-svelte';

	interface Props {
		defaults: StudioWebDefaults;
		contentTypes: ContentTypeMeta[];
		spaces: MeSpace[];
	}

	let { defaults = $bindable(), contentTypes, spaces }: Props = $props();

	let open = $state(false);
	let saved = $state(false);

	const categories = $derived(
		contentTypes.find((ct) => ct.value === defaults.default_content_type)?.categories ?? []
	);

	const typeItems = $derived(contentTypes.map((ct) => ({ id: ct.value, label: ct.label })));
	const categoryItems = $derived(categories.map((c) => ({ id: c.Slug, label: c.Label })));
	const spaceItems = $derived(spaces.map((s) => ({ id: s.id, label: s.name })));
	const statusItems = ['draft', 'in_review', 'published', 'scheduled'].map((s) => ({ id: s, label: s }));
	const visibilityItems = ['public', 'members', 'private'].map((v) => ({ id: v, label: v }));

	const inputCls =
		'rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-xs text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]';
	const emptyMsg = t('common.no_results', 'No results found');

	function labelFor(items: { id: string; label: string }[], id: string): string {
		return items.find((i) => i.id === id)?.label ?? '';
	}

	function save() {
		saveStudioDefaults(defaults);
		saved = true;
		setTimeout(() => (saved = false), 2000);
	}
</script>

{#snippet acField(
	label: string,
	items: { id: string; label: string }[],
	selectedId: string,
	onPick: (id: string) => void
)}
	<div class="flex flex-col gap-1 text-xs text-[var(--color-muted)]">
		<span>{label}</span>
		<Autocomplete
			placeholder={label}
			emptyMessage={emptyMsg}
			fetcher={staticAutocompleteFetcher(items)}
			initialFetcher={staticAutocompleteFetcher(items)}
			selectedId={selectedId || null}
			selectedLabel={labelFor(items, selectedId)}
			onSelect={(item) => onPick(item.id)}
			onClear={() => onPick('')}
		/>
	</div>
{/snippet}

<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
	<button
		type="button"
		onclick={() => (open = !open)}
		class="flex w-full items-center justify-between px-4 py-2.5 text-sm font-medium text-[var(--color-text)]"
		aria-expanded={open}
	>
		{t('studio.prompts.defaults', 'Project defaults')}
		{#if open}<ChevronUp size={16} />{:else}<ChevronDown size={16} />{/if}
	</button>

	{#if open}
		<div class="flex flex-wrap items-end gap-3 border-t border-[var(--color-border)] px-4 py-3">
			<div class="w-44">
				{@render acField(t('studio.prompts.content_type', 'Content type'), typeItems, defaults.default_content_type ?? '', (id) => {
					defaults.default_content_type = id;
					defaults.default_guide_category = '';
				})}
			</div>

			{#if categories.length > 0}
				<div class="w-44">
					{@render acField(t('studio.prompts.guide_category', 'Guide category'), categoryItems, defaults.default_guide_category ?? '', (id) => (defaults.default_guide_category = id))}
				</div>
			{/if}

			<div class="w-44">
				{@render acField(t('studio.prompts.space', 'Space'), spaceItems, defaults.default_space_id ?? '', (id) => (defaults.default_space_id = id))}
			</div>

			<div class="w-40">
				{@render acField(t('studio.prompts.status', 'Status'), statusItems, defaults.default_status ?? '', (id) => (defaults.default_status = id))}
			</div>

			<div class="w-40">
				{@render acField(t('studio.prompts.visibility', 'Visibility'), visibilityItems, defaults.default_visibility ?? '', (id) => (defaults.default_visibility = id))}
			</div>

			<label class="flex flex-col gap-1 text-xs text-[var(--color-muted)]">
				{t('studio.prompts.tags', 'Tags (comma-separated)')}
				<input
					type="text"
					class={inputCls}
					value={defaults.default_tags?.join(', ') ?? ''}
					oninput={(e) => {
						defaults.default_tags = e.currentTarget.value
							.split(',')
							.map((x) => x.trim())
							.filter(Boolean);
					}}
				/>
			</label>

			<button
				type="button"
				onclick={save}
				class="flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-white transition-colors hover:opacity-90"
			>
				<Save size={14} />
				{saved ? t('studio.prompts.defaults_saved', 'Saved') : t('studio.prompts.defaults_save', 'Save defaults')}
			</button>
		</div>
	{/if}
</div>
