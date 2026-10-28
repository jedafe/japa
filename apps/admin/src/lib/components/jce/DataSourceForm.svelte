<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { untrack } from 'svelte';
	import type { DataSource } from '$lib/types/jce';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

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

	function isValidUUID(s: string): boolean {
		if (!s) return true;
		const re = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
		return re.test(s.trim());
	}

	function configError(s: string): string {
		if (!s.trim()) return '';
		try {
			JSON.parse(s);
			return '';
		} catch (e) {
			return e instanceof Error ? e.message : t('data_sources.config_invalid', 'Config JSON is invalid');
		}
	}

	let validation = $derived({
		entity_id: value.entity_id && !isValidUUID(value.entity_id)
			? t('data_sources.entity_id_invalid', 'Entity ID must be a valid UUID')
			: '',
		config: configError(configInput)
	});

	let hasErrors = $derived(!!validation.entity_id || !!validation.config);

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
			<AutocompleteSelect
				id="ds-source-kind"
				bind:value={value.source_kind}
				options={sourceKindOptions}
				class="w-full"
			/>
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
			pattern="[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}"
			class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
			style="border-color: {validation.entity_id ? 'var(--color-danger)' : 'var(--color-border)'}; background-color: var(--color-surface); color: var(--color-foreground)"
			value={value.entity_id ?? ''}
			oninput={(e) => (value.entity_id = e.currentTarget.value)}
			aria-invalid={validation.entity_id ? 'true' : 'false'}
		/>
		{#if validation.entity_id}
			<p class="mt-1 text-xs" style="color: var(--color-danger)">{validation.entity_id}</p>
		{:else}
		{/if}
	</div>

	<div>
		<label for="ds-config" class="block text-sm font-medium" style="color: var(--color-foreground)">
			{t('data_sources.config', 'Config (JSON)')}
		</label>
		<textarea
			id="ds-config"
			rows="6"
			class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
			style="border-color: {validation.config ? 'var(--color-danger)' : 'var(--color-border)'}; background-color: var(--color-surface); color: var(--color-foreground)"
			bind:value={configInput}
			aria-invalid={validation.config ? 'true' : 'false'}
		></textarea>
		{#if validation.config}
			<p class="mt-1 text-xs" style="color: var(--color-danger)">{validation.config}</p>
		{:else}
			<HintIcon text={t('data_sources.config_hint', 'Optional JSON configuration object')} />
		{/if}
	</div>
<