<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { untrack } from 'svelte';
	import type { DataSource } from '$lib/types/jce';

	let {
		value = $bindable<Record<string, any>>({}),
		editing = $bindable<DataSource | null>(null)
	}: {
		value?: Record<string, any>;
		editing?: DataSource | null;
	} = $props();

	let configInput = $state('');

	const sourceKindOptions = $derived([
		{ value: 'official', label: t('data_sources.official', 'Official') },
		{ value: 'third_party', label: t('data_sources.third_party', 'Third party') },
		{ value: 'ai_estimate', label: t('data_sources.ai_estimate', 'AI estimate') }
	]);

	function configToString(cfg: unknown): string {
		if (cfg == null) return '';
		if (typeof cfg === 'string') return cfg;
		try {
			return JSON.stringify(cfg, null, 2);
		} catch {
			return '';
		}
	}

	$effect(() => {
		if (editing) {
			const cfg = configToString(editing.config);
			configInput = cfg;
			untrack(() => {
				value.config = cfg;
			});
		} else {
			configInput = '';
			untrack(() => {
				value.config = '';
				if (!value.source_kind) value.source_kind = 'official';
				if (value.trust_level == null || value.trust_level === '') value.trust_level = 3;
			});
		}
	});

	$effect(() => {
		value.config = configInput;
	});
</script>

<div class="space-y-4">
	<div>
		<label for="ds-source-name" class="block text-sm font-medium" style="color: var(--color-foreground)">
			{t('data_sources.source_name', 'Source name')}
			<span style="color: var(--color-danger)">*</span>
		</label>
		<input
			id="ds-source-name"
			type="text"
			class="mt-1 w-full rounded-lg border p-2 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
			bind:value={value.source_name}
		/>
	</div>

	<div class="grid grid-cols-2 gap-4">
		<div>
			<label for="ds-source-kind" class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t('data_sources.source_kind', 'Kind')}
				<span style="color: var(--color-danger)">*</span>
			</label>
			<select
				id="ds-source-kind"
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				bind:value={value.source_kind}
			>
				{#each sourceKindOptions as option (option.value)}
					<option value={option.value}>{option.label}</option>
				{/each}
			</select>
		</div>

		<div>
			<label for="ds-trust-level" class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t('data_sources.trust_level', 'Trust level')}
			</label>
			<input
				id="ds-trust-level"
				type="number"
				min={1}
				max={5}
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				bind:value={value.trust_level}
			/>
		</div>
	</div>

	<div>
		<label for="ds-source-url" class="block text-sm font-medium" style="color: var(--color-foreground)">
			{t('data_sources.source_url', 'Source URL')}
		</label>
		<input
			id="ds-source-url"
			type="url"
			class="mt-1 w-full rounded-lg border p-2 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
			value={value.source_url ?? ''}
			oninput={(e) => (value.source_url = e.currentTarget.value)}
		/>
	</div>

	<div class="grid grid-cols-2 gap-4">
		<div>
			<label for="ds-entity-type" class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t('data_sources.entity_type', 'Entity type')}
			</label>
			<input
				id="ds-entity-type"
				type="text"
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				value={value.entity_type ?? ''}
				oninput={(e) => (value.entity_type = e.currentTarget.value)}
			/>
		</div>

		<div>
			<label for="ds-dimension-slug" class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t('data_sources.dimension_slug', 'Dimension slug')}
			</label>
			<input
				id="ds-dimension-slug"
				type="text"
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				value={value.dimension_slug ?? ''}
				oninput={(e) => (value.dimension_slug = e.currentTarget.value)}
			/>
		</div>
	</div>

	<div>
		<label for="ds-entity-id" class="block text-sm font-medium" style="color: var(--color-foreground)">
			{t('data_sources.entity_id', 'Entity ID')}
		</label>
		<input
			id="ds-entity-id"
			type="text"
			class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
			style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
			value={value.entity_id ?? ''}
			oninput={(e) => (value.entity_id = e.currentTarget.value)}
		/>
		<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('data_sources.entity_id_hint', 'Optional UUID of an entity this source applies to')}</p>
	</div>

	<div>
		<label for="ds-config" class="block text-sm font-medium" style="color: var(--color-foreground)">
			{t('data_sources.config', 'Config (JSON)')}
		</label>
		<textarea
			id="ds-config"
			rows="6"
			class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
			style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
			bind:value={configInput}
		></textarea>
		<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('data_sources.config_hint', 'Optional JSON configuration object')}</p>
	</div>
</div>
