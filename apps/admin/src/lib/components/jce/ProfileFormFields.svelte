<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	type Goal = { key: string; weight: number };

	interface Props {
		value: Record<string, any>;
		editing?: Record<string, any> | null;
	}

	let { value, editing }: Props = $props();

	function ensureGoals(): Goal[] {
		if (!Array.isArray(value.goals)) {
			value.goals = [];
		}
		return value.goals;
	}

	function ensurePreferences(): Record<string, number> {
		if (typeof value.preferences !== 'object' || value.preferences === null) {
			value.preferences = {};
		}
		return value.preferences;
	}

	function ensureDemographics(): Record<string, any> {
		if (typeof value.demographics !== 'object' || value.demographics === null) {
			value.demographics = {};
		}
		return value.demographics;
	}

	function addGoal() {
		ensureGoals().push({ key: '', weight: 1 });
		value.goals = value.goals;
	}

	function removeGoal(index: number) {
		ensureGoals().splice(index, 1);
		value.goals = value.goals;
	}

	function preferenceRows(): { key: string; weight: number }[] {
		const prefs = ensurePreferences();
		return Object.entries(prefs).map(([key, weight]) => ({ key, weight: Number(weight) || 0 }));
	}

	function addPreference() {
		const prefs = ensurePreferences();
		prefs[''] = 1;
		value.preferences = prefs;
	}

	function removePreference(key: string) {
		const prefs = ensurePreferences();
		delete prefs[key];
		value.preferences = prefs;
	}

	function updatePreferenceKey(oldKey: string, newKey: string) {
		const prefs = ensurePreferences();
		if (oldKey !== newKey) {
			const weight = prefs[oldKey] ?? 0;
			delete prefs[oldKey];
			prefs[newKey.trim()] = weight;
		}
		value.preferences = prefs;
	}

	function updatePreferenceWeight(key: string, weight: number) {
		const prefs = ensurePreferences();
		prefs[key] = weight;
		value.preferences = prefs;
	}

	function updateDemographicsField(key: string, raw: string) {
		const d = ensureDemographics();
		if (raw === '') {
			delete d[key];
		} else if (['age', 'budget_min', 'budget_max'].includes(key)) {
			const n = Number(raw);
			d[key] = Number.isNaN(n) ? raw : n;
		} else {
			d[key] = raw;
		}
		value.demographics = d;
	}

	function demographicsValue(key: string): string {
		const d = ensureDemographics();
		const v = d[key];
		if (v === undefined || v === null) return '';
		return String(v);
	}

	const canCreateTemplate = $derived(hasPermission('comparison.edit_any'));
</script>

<div class="space-y-4">
	<div>
		<h3 class="mb-2 text-sm font-medium" style="color: var(--color-foreground)">
			{t('comparison_profiles.goals', 'Goals')}
		</h3>
		<div class="space-y-2">
			{#each ensureGoals() as goal, i (i)}
				<div class="flex items-center gap-2">
					<input
						type="text"
						class="flex-1 rounded-lg border p-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						placeholder={t('comparison_profiles.goal_key', 'Goal key')}
						bind:value={goal.key}
					/>
					<input
						type="number"
						step="0.1"
						class="w-24 rounded-lg border p-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={goal.weight}
					/>
					<button
						type="button"
						class="rounded-lg border p-2 text-sm"
						style="border-color: var(--color-border); color: var(--color-danger)"
						onclick={() => removeGoal(i)}
					>
						{t('common.remove', 'Remove')}
					</button>
				</div>
			{/each}
		</div>
		<button
			type="button"
			class="mt-2 rounded-lg border px-3 py-1.5 text-sm"
			style="border-color: var(--color-border); color: var(--color-foreground)"
			onclick={addGoal}
		>
			{t('common.add', 'Add goal')}
		</button>
	</div>

	<div>
		<h3 class="mb-2 text-sm font-medium" style="color: var(--color-foreground)">
			{t('comparison_profiles.preferences', 'Preferences')}
		</h3>
		<p class="mb-2 text-xs" style="color: var(--color-muted)">
			{t('comparison_profiles.preferences_hint', 'Factor-level weight overrides keyed by factor key')}
		</p>
		<div class="space-y-2">
			{#each preferenceRows() as row (row.key)}
				<div class="flex items-center gap-2">
					<input
						type="text"
						class="flex-1 rounded-lg border p-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						placeholder={t('comparison_profiles.preference_key', 'Factor key')}
						value={row.key}
						onchange={(e) => updatePreferenceKey(row.key, e.currentTarget.value)}
					/>
					<input
						type="number"
						step="0.05"
						class="w-24 rounded-lg border p-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						value={row.weight}
						onchange={(e) => updatePreferenceWeight(row.key, Number(e.currentTarget.value))}
					/>
					<button
						type="button"
						class="rounded-lg border p-2 text-sm"
						style="border-color: var(--color-border); color: var(--color-danger)"
						onclick={() => removePreference(row.key)}
					>
						{t('common.remove', 'Remove')}
					</button>
				</div>
			{/each}
		</div>
		<button
			type="button"
			class="mt-2 rounded-lg border px-3 py-1.5 text-sm"
			style="border-color: var(--color-border); color: var(--color-foreground)"
			onclick={addPreference}
		>
			{t('common.add', 'Add preference')}
		</button>
	</div>

	<div class="grid gap-4 sm:grid-cols-2">
		<label class="block space-y-1">
			<span class="text-xs" style="color: var(--color-muted)">{t('comparison_profiles.demographics.age', 'Age')}</span>
			<input
				type="text"
				class="w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={demographicsValue('age')}
				onchange={(e) => updateDemographicsField('age', e.currentTarget.value)}
			/>
		</label>
		<label class="block space-y-1">
			<span class="text-xs" style="color: var(--color-muted)">{t('comparison_profiles.demographics.occupation', 'Occupation')}</span>
			<input
				type="text"
				class="w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={demographicsValue('occupation')}
				onchange={(e) => updateDemographicsField('occupation', e.currentTarget.value)}
			/>
		</label>
		<label class="block space-y-1">
			<span class="text-xs" style="color: var(--color-muted)">{t('comparison_profiles.demographics.family_status', 'Family status')}</span>
			<input
				type="text"
				class="w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={demographicsValue('family_status')}
				onchange={(e) => updateDemographicsField('family_status', e.currentTarget.value)}
			/>
		</label>
		<label class="block space-y-1">
			<span class="text-xs" style="color: var(--color-muted)">{t('comparison_profiles.demographics.language_proficiency', 'Language proficiency')}</span>
			<input
				type="text"
				class="w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={demographicsValue('language_proficiency')}
				onchange={(e) => updateDemographicsField('language_proficiency', e.currentTarget.value)}
			/>
		</label>
		<label class="block space-y-1">
			<span class="text-xs" style="color: var(--color-muted)">{t('comparison_profiles.demographics.budget_min', 'Budget min')}</span>
			<input
				type="number"
				step="0.01"
				class="w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={demographicsValue('budget_min')}
				onchange={(e) => updateDemographicsField('budget_min', e.currentTarget.value)}
			/>
		</label>
		<label class="block space-y-1">
			<span class="text-xs" style="color: var(--color-muted)">{t('comparison_profiles.demographics.budget_max', 'Budget max')}</span>
			<input
				type="number"
				step="0.01"
				class="w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={demographicsValue('budget_max')}
				onchange={(e) => updateDemographicsField('budget_max', e.currentTarget.value)}
			/>
		</label>
		<label class="block space-y-1 sm:col-span-2">
			<span class="text-xs" style="color: var(--color-muted)">{t('comparison_profiles.demographics.budget_currency', 'Budget currency')}</span>
			<input
				type="text"
				class="w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={demographicsValue('budget_currency')}
				onchange={(e) => updateDemographicsField('budget_currency', e.currentTarget.value)}
			/>
		</label>
	</div>

	<div class="grid grid-cols-2 gap-4 rounded-lg border p-3" style="border-color: var(--color-border)">
		{#if canCreateTemplate}
			<div class="flex items-center gap-2">
				<input
					id="profile-template"
					type="checkbox"
					checked={!!value.is_template}
					onchange={(e) => { value.is_template = e.currentTarget.checked; }}
				/>
				<label for="profile-template" class="text-sm" style="color: var(--color-foreground)">
					{t('comparison_profiles.is_template', 'Template')}
				</label>
			</div>
		{/if}
		<div class="flex items-center gap-2">
			<input
				id="profile-default"
				type="checkbox"
				checked={!!value.is_default}
				onchange={(e) => { value.is_default = e.currentTarget.checked; }}
			/>
			<label for="profile-default" class="text-sm" style="color: var(--color-foreground)">
				{t('comparison_profiles.is_default', 'Default')}
			</label>
		</div>
	</div>
</div>
