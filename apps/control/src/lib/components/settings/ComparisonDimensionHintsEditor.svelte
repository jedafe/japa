<script lang="ts">
	// ComparisonDimensionHintsEditor — structured editor for the
	// comparison.auto_dimension_hints system setting.
	//
	// The setting stores a JSON object with two fields:
	//   - default: ordered list of dimension labels always suggested
	//   - groups: keyword-matched groups that append or prepend extra dimensions
	//
	// Each group may also carry a `name` field for display in this editor; the
	// backend service ignores the name when deriving dimensions, so it is safe to
	// persist as part of the JSON value.
	//
	// Ref: japaconnect-engineering/conventions/system-settings-catalog.md
	// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
	import { t } from '$lib/stores/i18n.svelte';
	import { ChevronDown, Plus, Trash2, X } from 'lucide-svelte';
	import { Switch } from 'bits-ui';
	import { comparisonHintPresets, type ComparisonHintPreset } from './comparisonPresets';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	interface HintGroup {
		name?: string;
		hints: string[];
		dimensions: string[];
		prepend: boolean;
	}

	interface HintConfig {
		default: string[];
		groups: HintGroup[];
	}

	interface LocalGroup extends HintGroup {
		id: string;
	}

	interface LocalConfig {
		default: string[];
		groups: LocalGroup[];
	}

	let {
		value,
		disabled = false,
		onchange
	}: {
		value: string;
		disabled?: boolean;
		onchange: (next: string) => void;
	} = $props();

	let config = $state<LocalConfig>({ default: [], groups: [] });
	let defaultInput = $state('');
	let groupInputs = $state<Record<string, { hint: string; dim: string }>>({});
	let selectedPreset = $state<string>('');
	let openStates = $state<Record<string, boolean>>({});
	let editingNames = $state<Record<string, string>>({});
	let lastEmitted = '';

	$effect(() => {
		// Compare the raw JSON string, not JSON.stringify(value). lastEmitted is
		// already a JSON string, so re-stringifying the prop would never match.
		const current = value ?? '{}';
		if (current === lastEmitted) return;
		defaultInput = '';
		groupInputs = {};
		selectedPreset = '';
		// Parse and update in a local variable first. Writing `config` and then
		// reading `config.groups` in the same effect creates a self-dependency and
		// triggers an infinite Svelte 5 effect loop.
		const nextConfig = parseValue(value);
		config = nextConfig;
		openStates = Object.fromEntries(nextConfig.groups.map((g) => [g.id, false]));
		editingNames = Object.fromEntries(nextConfig.groups.map((g) => [g.id, g.name ?? '']));
	});

	function newId(): string {
		if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
			return crypto.randomUUID();
		}
		return Math.random().toString(36).slice(2);
	}

	function parseValue(v: string): LocalConfig {
		if (!v) return { default: [], groups: [] };
		let raw = v.trim();
		// The DB may return the JSON object wrapped in a JSON string (e.g. when the
		// value was saved as a jsonb string literal). Unwrap one layer of quoting
		// before parsing the actual object.
		if (raw.length >= 2 && raw.startsWith('"') && raw.endsWith('"')) {
			try {
				const inner = JSON.parse(raw);
				if (typeof inner === 'string') raw = inner.trim();
			} catch {
				/* fall through to normal parse */
			}
		}
		try {
			const parsed = JSON.parse(raw);
			if (typeof parsed !== 'object' || parsed === null) return { default: [], groups: [] };
			return {
				default: isStringArray(parsed.default),
				groups: isGroupArray(parsed.groups)
			};
		} catch {
			return { default: [], groups: [] };
		}
	}

	function isStringArray(x: unknown): string[] {
		return Array.isArray(x) ? x.filter((item): item is string => typeof item === 'string') : [];
	}

	function isGroupArray(x: unknown): LocalGroup[] {
		return Array.isArray(x) ? x.map(normalizeGroup) : [];
	}

	function normalizeGroup(g: unknown): LocalGroup {
		if (typeof g !== 'object' || g === null) {
			return { id: newId(), hints: [], dimensions: [], prepend: false };
		}
		const obj = g as Record<string, unknown>;
		return {
			id: newId(),
			name: typeof obj.name === 'string' ? obj.name.trim() : undefined,
			hints: isStringArray(obj.hints),
			dimensions: isStringArray(obj.dimensions),
			prepend: typeof obj.prepend === 'boolean' ? obj.prepend : false
		};
	}

	function stripIds(cfg: LocalConfig): HintConfig {
		return {
			default: cfg.default,
			groups: cfg.groups.map(({ id, ...rest }) => rest)
		};
	}

	function emit(next: LocalConfig) {
		config = next;
		const stripped = stripIds(next);
		const serialized = JSON.stringify(stripped);
		lastEmitted = serialized;
		onchange(serialized);
	}

	function addDefault() {
		const val = defaultInput.trim();
		if (!val) return;
		defaultInput = '';
		emit({ ...config, default: [...config.default, val] });
	}

	function removeDefault(index: number) {
		emit({ ...config, default: config.default.filter((_, i) => i !== index) });
	}

	function addGroup(overrides: Partial<HintGroup> = {}) {
		const id = newId();
		groupInputs = { ...groupInputs, [id]: { hint: '', dim: '' } };
		openStates = { ...openStates, [id]: true };
		editingNames = { ...editingNames, [id]: overrides.name ?? '' };
		emit({
			...config,
			groups: [
				...config.groups,
				{
					id,
					name: overrides.name,
					hints: [...(overrides.hints ?? [])],
					dimensions: [...(overrides.dimensions ?? [])],
					prepend: overrides.prepend ?? false
				}
			]
		});
	}

	function removeGroup(id: string) {
		const { [id]: _, ...rest } = groupInputs;
		groupInputs = rest;
		const { [id]: __, ...restOpen } = openStates;
		openStates = restOpen;
		const { [id]: ___, ...restNames } = editingNames;
		editingNames = restNames;
		emit({ ...config, groups: config.groups.filter((g) => g.id !== id) });
	}

	function toggleGroup(id: string) {
		openStates = { ...openStates, [id]: !openStates[id] };
	}

	function updateGroup(id: string, patch: Partial<HintGroup>) {
		emit({
			...config,
			groups: config.groups.map((g) => (g.id === id ? { ...g, ...patch } : g))
		});
	}

	function setGroupInput(id: string, field: 'hint' | 'dim', value: string) {
		const existing = groupInputs[id] ?? { hint: '', dim: '' };
		groupInputs = { ...groupInputs, [id]: { ...existing, [field]: value } };
	}

	function addGroupHint(id: string) {
		const val = (groupInputs[id]?.hint ?? '').trim();
		if (!val) return;
		setGroupInput(id, 'hint', '');
		emit({
			...config,
			groups: config.groups.map((g) => (g.id === id ? { ...g, hints: [...g.hints, val] } : g))
		});
	}

	function removeGroupHint(id: string, index: number) {
		emit({
			...config,
			groups: config.groups.map((g) =>
				g.id === id ? { ...g, hints: g.hints.filter((_, i) => i !== index) } : g
			)
		});
	}

	function addGroupDimension(id: string) {
		const val = (groupInputs[id]?.dim ?? '').trim();
		if (!val) return;
		setGroupInput(id, 'dim', '');
		emit({
			...config,
			groups: config.groups.map((g) => (g.id === id ? { ...g, dimensions: [...g.dimensions, val] } : g))
		});
	}

	function removeGroupDimension(id: string, index: number) {
		emit({
			...config,
			groups: config.groups.map((g) =>
				g.id === id ? { ...g, dimensions: g.dimensions.filter((_, i) => i !== index) } : g
			)
		});
	}

	function groupDisplayName(group: LocalGroup, index: number): string {
		return (
			editingNames[group.id]?.trim() ||
			group.name?.trim() ||
			t('settings.comparison_group_label', 'Group {n}').replace('{n}', String(index + 1))
		);
	}

	function findPreset(name: string): ComparisonHintPreset | undefined {
		return comparisonHintPresets.find((p) => p.name === name);
	}

	function addPreset() {
		const name = selectedPreset.trim();
		if (!name) return;
		const preset = findPreset(name);
		if (!preset) return;
		addGroup(preset);
		selectedPreset = '';
	}

	function addAllPresets() {
		const newGroups: LocalGroup[] = comparisonHintPresets.map((preset) => ({
			id: newId(),
			name: preset.name,
			hints: [...preset.hints],
			dimensions: [...preset.dimensions],
			prepend: preset.prepend ?? false
		}));
		groupInputs = comparisonHintPresets.reduce(
			(acc, _, i) => ({ ...acc, [newGroups[i].id]: { hint: '', dim: '' } }),
			{ ...groupInputs }
		);
		openStates = {
			...openStates,
			...Object.fromEntries(newGroups.map((g) => [g.id, true]))
		};
		editingNames = {
			...editingNames,
			...Object.fromEntries(newGroups.map((g) => [g.id, g.name ?? '']))
		};
		emit({
			...config,
			groups: [...config.groups, ...newGroups]
		});
		selectedPreset = '';
	}
</script>

<div class="w-full max-w-2xl space-y-4">
	<!-- Default dimensions -->
	<div class="space-y-2">
		<h4 class="text-foreground text-sm font-medium">
			{t('settings.comparison_default_dimensions', 'Default dimensions')}
		</h4>
		{#if config.default.length === 0}
			<p class="text-muted-foreground text-xs">
				{t(
					'settings.comparison_default_empty',
					'No default dimensions. Add dimensions suggested for every comparison.'
				)}
			</p>
		{:else}
			<div class="flex flex-wrap gap-1">
				{#each config.default as dim, i (i)}
					<span
						class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px]"
					>
						{dim}
						<button
							type="button"
							class="hover:text-destructive disabled:opacity-30"
							disabled={disabled}
							onclick={() => removeDefault(i)}
							aria-label={t('common.remove', 'Remove')}
						>
							<X class="h-3 w-3" />
						</button>
					</span>
				{/each}
			</div>
		{/if}
		<div class="flex items-center gap-2">
			<input
				type="text"
				value={defaultInput}
				placeholder={t('settings.comparison_default_placeholder', 'Add a default dimension...')}
				disabled={disabled}
				oninput={(e) => {
					defaultInput = (e.currentTarget as HTMLInputElement).value;
				}}
				onkeydown={(e) => {
					if (e.key === 'Enter') {
						e.preventDefault();
						addDefault();
					}
				}}
				class="border-border-input bg-background text-foreground w-64 rounded-lg border px-3 py-1.5 text-sm transition-all hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus disabled:opacity-50"
			/>
			<button
				type="button"
				class="border-border-input bg-background-alt hover:bg-muted text-muted-foreground flex items-center gap-1 rounded-lg border px-2 py-1.5 text-xs transition-colors disabled:opacity-50"
				disabled={disabled}
				onclick={addDefault}
			>
				<Plus class="h-3.5 w-3.5" />
				{t('settings.comparison_add', 'Add')}
			</button>
		</div>
	</div>

	<!-- Hint groups -->
	<div class="space-y-2">
		<h4 class="text-foreground text-sm font-medium">
			{t('settings.comparison_hint_groups', 'Hint groups')}
		</h4>
		{#if config.groups.length === 0}
			<p class="text-muted-foreground text-xs">
				{t(
					'settings.comparison_groups_empty',
					'No hint groups. Add a group to suggest dimensions when item names contain certain keywords.'
				)}
			</p>
		{/if}
		<div class="space-y-2">
			{#each config.groups as group, gi (group.id)}
				<div class="border-border-input bg-background rounded-xl border overflow-hidden">
					<div class="flex items-center gap-2 px-3 py-2">
						<button
							type="button"
							class="flex flex-1 items-center gap-2 text-left text-xs font-medium text-foreground transition-colors disabled:opacity-50"
							disabled={disabled}
							aria-expanded={openStates[group.id] ?? false}
							onclick={() => toggleGroup(group.id)}
						>
							<ChevronDown
								class="h-4 w-4 shrink-0 transition-transform {openStates[group.id] ? 'rotate-180' : ''}"
							/>
							<span class="truncate">{groupDisplayName(group, gi)}</span>
						</button>
						<button
							type="button"
							class="text-muted-foreground hover:text-destructive hover:bg-red-500/10 rounded p-1 transition-colors disabled:opacity-30"
							disabled={disabled}
							onclick={() => removeGroup(group.id)}
							aria-label={t('common.remove', 'Remove')}
						>
							<Trash2 class="h-3.5 w-3.5" />
						</button>
					</div>
					{#if openStates[group.id]}
						<div class="space-y-3 border-t border-border-input p-3">
							<!-- Group name -->
							<div class="space-y-1.5">
								<label
									for="comparison-group-name-{group.id}"
									class="text-muted-foreground text-[11px] font-medium"
								>
									{t('settings.comparison_group_name', 'Group name')}
								</label>
								<input
									id="comparison-group-name-{group.id}"
									type="text"
									value={editingNames[group.id] ?? group.name ?? ''}
									placeholder={t('settings.comparison_group_label', 'Group {n}').replace('{n}', String(gi + 1))}
									disabled={disabled}
									oninput={(e) => {
										editingNames = {
											...editingNames,
											[group.id]: (e.currentTarget as HTMLInputElement).value
										};
									}}
									onchange={(e) => {
										const name = (e.currentTarget as HTMLInputElement).value;
										updateGroup(group.id, { name: name.trim() ? name : undefined });
									}}
									class="border-border-input bg-background text-foreground w-full rounded-lg border px-3 py-1.5 text-sm font-medium transition-all hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus disabled:opacity-50"
								/>
							</div>

							<!-- Keywords -->
							<div class="space-y-1.5">
								<span class="text-muted-foreground text-[11px] font-medium">
									{t('settings.comparison_keywords', 'Keywords')}
								</span>
								{#if group.hints.length > 0}
									<div class="flex flex-wrap gap-1">
										{#each group.hints as hint, hi (hi)}
											<span
												class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px]"
											>
												{hint}
												<button
													type="button"
													class="hover:text-destructive disabled:opacity-30"
													disabled={disabled}
													onclick={() => removeGroupHint(group.id, hi)}
													aria-label={t('common.remove', 'Remove')}
												>
													<X class="h-3 w-3" />
												</button>
											</span>
										{/each}
									</div>
								{/if}
								<div class="flex items-center gap-2">
									<input
										type="text"
										value={groupInputs[group.id]?.hint ?? ''}
										placeholder={t('settings.comparison_keyword_placeholder', 'Add keyword...')}
										disabled={disabled}
										oninput={(e) => {
											setGroupInput(group.id, 'hint', (e.currentTarget as HTMLInputElement).value);
										}}
										onkeydown={(e) => {
											if (e.key === 'Enter') {
												e.preventDefault();
												addGroupHint(group.id);
											}
										}}
										class="border-border-input bg-background text-foreground w-48 rounded-lg border px-3 py-1.5 text-sm transition-all hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus disabled:opacity-50"
									/>
									<button
										type="button"
										class="border-border-input bg-background-alt hover:bg-muted text-muted-foreground flex items-center gap-1 rounded-lg border px-2 py-1.5 text-xs transition-colors disabled:opacity-50"
										disabled={disabled}
										onclick={() => addGroupHint(group.id)}
									>
										<Plus class="h-3.5 w-3.5" />
										{t('settings.comparison_add', 'Add')}
									</button>
								</div>
							</div>

							<!-- Suggested dimensions -->
							<div class="space-y-1.5">
								<span class="text-muted-foreground text-[11px] font-medium">
									{t('settings.comparison_group_dimensions', 'Suggested dimensions')}
								</span>
								{#if group.dimensions.length > 0}
									<div class="flex flex-wrap gap-1">
										{#each group.dimensions as dim, di (di)}
											<span
												class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px]"
											>
												{dim}
												<button
													type="button"
													class="hover:text-destructive disabled:opacity-30"
													disabled={disabled}
													onclick={() => removeGroupDimension(group.id, di)}
													aria-label={t('common.remove', 'Remove')}
												>
													<X class="h-3 w-3" />
												</button>
											</span>
										{/each}
									</div>
								{/if}
								<div class="flex items-center gap-2">
									<input
										type="text"
										value={groupInputs[group.id]?.dim ?? ''}
										placeholder={t('settings.comparison_dimension_placeholder', 'Add dimension...')}
										disabled={disabled}
										oninput={(e) => {
											setGroupInput(group.id, 'dim', (e.currentTarget as HTMLInputElement).value);
										}}
										onkeydown={(e) => {
											if (e.key === 'Enter') {
												e.preventDefault();
												addGroupDimension(group.id);
											}
										}}
										class="border-border-input bg-background text-foreground w-48 rounded-lg border px-3 py-1.5 text-sm transition-all hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus disabled:opacity-50"
									/>
									<button
										type="button"
										class="border-border-input bg-background-alt hover:bg-muted text-muted-foreground flex items-center gap-1 rounded-lg border px-2 py-1.5 text-xs transition-colors disabled:opacity-50"
										disabled={disabled}
										onclick={() => addGroupDimension(group.id)}
									>
										<Plus class="h-3.5 w-3.5" />
										{t('settings.comparison_add', 'Add')}
									</button>
								</div>
							</div>

							<!-- Prepend toggle -->
							<div class="flex items-center gap-2">
								<Switch.Root
									id="comparison-prepend-{group.id}"
									checked={group.prepend}
									onCheckedChange={(checked) => {
										updateGroup(group.id, { prepend: checked });
									}}
									disabled={disabled}
									class="bg-muted data-[state=checked]:bg-emerald-500 peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full px-[2px] transition-colors disabled:cursor-not-allowed disabled:opacity-50"
								>
									<Switch.Thumb
										class="bg-background pointer-events-none block size-4 shrink-0 rounded-full shadow-lg transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0"
									/>
								</Switch.Root>
								<label
									for="comparison-prepend-{group.id}"
									class="text-muted-foreground cursor-pointer text-xs"
								>
									{t('settings.comparison_prepend', 'Prepend before defaults')}
								</label>
							</div>
						</div>
					{/if}
				</div>
			{/each}
		</div>
		<button
			type="button"
			class="border-border-input bg-background-alt hover:bg-muted text-muted-foreground flex w-full items-center justify-center gap-2 rounded-lg border border-dashed px-3 py-2 text-sm font-medium transition-colors disabled:opacity-50"
			disabled={disabled}
			onclick={() => addGroup()}
		>
			<Plus class="h-4 w-4" />
			{t('settings.comparison_add_group', 'Add hint group')}
		</button>
	</div>

	<!-- Presets -->
	<div class="space-y-2 border-t border-dashed border-border-input pt-4">
		<h4 class="text-foreground text-sm font-medium">
			{t('settings.comparison_presets', 'JapaConnect presets')}
		</h4>
		<div class="flex flex-wrap items-center gap-2">
			<AutocompleteSelect
				id="comparison-presets"
				{disabled}
				value={selectedPreset}
				onValueChange={(v) => (selectedPreset = v)}
				options={[
					{ value: '', label: t('settings.comparison_select_preset', 'Select a preset…') },
					...comparisonHintPresets.map((p) => ({ value: p.name, label: p.name }))
				]}
				ariaLabel={t('settings.comparison_select_preset', 'Select a preset…')}
				class="w-64"
			/>
			<button
				type="button"
				class="border-border-input bg-background-alt hover:bg-muted text-muted-foreground flex items-center gap-1 rounded-lg border px-2 py-1.5 text-xs transition-colors disabled:opacity-50"
				disabled={disabled || !selectedPreset}
				onclick={addPreset}
			>
				<Plus class="h-3.5 w-3.5" />
				{t('settings.comparison_add_preset', 'Add preset')}
			</button>
			<button
				type="button"
				class="text-muted-foreground hover:text-foreground text-xs underline transition-colors disabled:opacity-50"
				disabled={disabled}
				onclick={addAllPresets}
			>
				{t('settings.comparison_add_all_presets', 'Add all')}
			</button>
		</div>
	</div>
</div>
