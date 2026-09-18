<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §6 — project-level
	// defaults persisted to localStorage (japa-studio:web-settings).
	import { t } from '$lib/i18n';
	import type { ContentTypeMeta, MeSpace } from '$lib/types';
	import type { StudioWebDefaults } from '$lib/studio/defaults';
	import { saveStudioDefaults } from '$lib/studio/defaults';
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

	const inputCls =
		'rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-xs text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]';

	function save() {
		saveStudioDefaults(defaults);
		saved = true;
		setTimeout(() => (saved = false), 2000);
	}
</script>

<div class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
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
			<label class="flex flex-col gap-1 text-xs text-[var(--color-muted)]">
				{t('studio.prompts.content_type', 'Content type')}
				<select bind:value={defaults.default_content_type} class={inputCls}>
					<option value="">{t('studio.prompts.none', '—')}</option>
					{#each contentTypes as ct (ct.value)}
						<option value={ct.value}>{ct.label}</option>
					{/each}
				</select>
			</label>

			{#if categories.length > 0}
				<label class="flex flex-col gap-1 text-xs text-[var(--color-muted)]">
					{t('studio.prompts.guide_category', 'Guide category')}
					<select bind:value={defaults.default_guide_category} class={inputCls}>
						<option value="">{t('studio.prompts.none', '—')}</option>
						{#each categories as cat (cat.Slug)}
							<option value={cat.Slug}>{cat.Label}</option>
						{/each}
					</select>
				</label>
			{/if}

			<label class="flex flex-col gap-1 text-xs text-[var(--color-muted)]">
				{t('studio.prompts.space', 'Space')}
				<select bind:value={defaults.default_space_id} class={inputCls}>
					<option value="">{t('studio.prompts.none', '—')}</option>
					{#each spaces as s (s.id)}
						<option value={s.id}>{s.name}</option>
					{/each}
				</select>
			</label>

			<label class="flex flex-col gap-1 text-xs text-[var(--color-muted)]">
				{t('studio.prompts.status', 'Status')}
				<select bind:value={defaults.default_status} class={inputCls}>
					<option value="">{t('studio.prompts.none', '—')}</option>
					{#each ['draft', 'in_review', 'published', 'scheduled'] as st (st)}
						<option value={st}>{st}</option>
					{/each}
				</select>
			</label>

			<label class="flex flex-col gap-1 text-xs text-[var(--color-muted)]">
				{t('studio.prompts.visibility', 'Visibility')}
				<select bind:value={defaults.default_visibility} class={inputCls}>
					<option value="">{t('studio.prompts.none', '—')}</option>
					{#each ['public', 'members', 'private'] as v (v)}
						<option value={v}>{v}</option>
					{/each}
				</select>
			</label>

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
