<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { t } from '$lib/i18n';
	import type { ComparisonProfile, ComparisonProfileInput, ProfileStat } from '$lib/types/jce';
	import { createProfile, updateProfile, searchSpaces, getProfileStats } from '$lib/api/jce';
	import JceSelect from '$lib/components/ui/JceSelect.svelte';
	import { Plus, Pencil, X, Save, User, Check } from 'lucide-svelte';

	interface Props {
		open: boolean;
		profiles: ComparisonProfile[];
		activeProfile: ComparisonProfile | null;
		scoreFactors?: { key: string; label?: string }[];
		allowedGoals?: { key: string; label?: string; default_weight?: number }[];
		/** Called when a profile is selected or successfully saved. */
		onsave?: (p: ComparisonProfile) => void | Promise<void>;
	}

	let {
		open = $bindable(),
		profiles,
		activeProfile = $bindable(),
	scoreFactors = [],
		allowedGoals = [],
		onsave
	}: Props = $props();

	type View = 'select' | 'edit';
	let view = $state<View>('select');

	type WeightedDraft = { key: string; weight: number };
	type Draft = {
		id?: string;
		title: string;
		source_space_id: string;
		source_space_name: string;
		target_space_ids: string[];
		target_space_names: Record<string, string>;
		age: string;
		occupation: string;
		family_status: string;
		language_proficiency: string;
		budget_min: string;
		budget_max: string;
		budget_currency: string;
		is_default: boolean;
		goals: WeightedDraft[];
		preferences: WeightedDraft[];
	};

	const emptyDraft: Draft = {
		title: '',
		source_space_id: '',
		source_space_name: '',
		target_space_ids: [],
		target_space_names: {},
		age: '',
		occupation: '',
		family_status: '',
		language_proficiency: '',
		budget_min: '',
		budget_max: '',
		budget_currency: '',
		is_default: false,
		goals: [],
		preferences: []
	};

	let draft = $state<Draft>({ ...emptyDraft });
	let saving = $state(false);
	let error = $state('');

	let sourceSearch = $state('');
	let sourceSuggestions = $state<{ id: string; name: string }[]>([]);
	let sourceSearchTimer: ReturnType<typeof setTimeout> | null = null;

	let targetSearch = $state('');
	let targetSuggestions = $state<{ id: string; name: string }[]>([]);
	let targetSearchTimer: ReturnType<typeof setTimeout> | null = null;

	let profileStats = $state<ProfileStat[]>([]);
	let profileStatsLoading = $state(false);

	async function fetchProfileStats() {
		if (!open || view !== 'select') return;
		profileStatsLoading = true;
		try {
			profileStats = await getProfileStats(false);
		} finally {
			profileStatsLoading = false;
		}
	}

	$effect(() => {
		if (open && view === 'select') {
			fetchProfileStats();
		}
	});

	async function fetchSourceSuggestions(query: string) {
		if (!query.trim()) {
			sourceSuggestions = [];
			return;
		}
		const spaces = await searchSpaces(query, undefined, 5);
		sourceSuggestions = spaces.filter((s) => !draft.source_space_id || s.id !== draft.source_space_id);
	}

	function onSourceInput(query: string) {
		sourceSearch = query;
		if (sourceSearchTimer) clearTimeout(sourceSearchTimer);
		sourceSearchTimer = setTimeout(() => fetchSourceSuggestions(query), 200);
	}

	function selectSource(space: { id: string; name: string }) {
		draft.source_space_id = space.id;
		draft.source_space_name = space.name;
		sourceSearch = '';
		sourceSuggestions = [];
	}

	function clearSource() {
		draft.source_space_id = '';
		draft.source_space_name = '';
		sourceSearch = '';
		sourceSuggestions = [];
	}

	async function fetchTargetSuggestions(query: string) {
		if (!query.trim()) {
			targetSuggestions = [];
			return;
		}
		const spaces = await searchSpaces(query, undefined, 5);
		targetSuggestions = spaces.filter((s) => !draft.target_space_ids.includes(s.id));
	}

	function onTargetInput(query: string) {
		targetSearch = query;
		if (targetSearchTimer) clearTimeout(targetSearchTimer);
		targetSearchTimer = setTimeout(() => fetchTargetSuggestions(query), 200);
	}

	function addTarget(space: { id: string; name: string }) {
		if (!draft.target_space_ids.includes(space.id)) {
			draft.target_space_ids = [...draft.target_space_ids, space.id];
			draft.target_space_names = { ...draft.target_space_names, [space.id]: space.name };
		}
		targetSearch = '';
		targetSuggestions = [];
	}

	function removeTarget(id: string) {
		draft.target_space_ids = draft.target_space_ids.filter((sid) => sid !== id);
		const names = { ...draft.target_space_names };
		delete names[id];
		draft.target_space_names = names;
	}

	function loadDraft(profile: ComparisonProfile | null, isNew: boolean) {
		sourceSearch = '';
		sourceSuggestions = [];
		targetSearch = '';
		targetSuggestions = [];

		if (!profile || isNew) {
			draft = { ...emptyDraft };
			return;
		}

		draft = {
			id: profile.id,
			title: profile.title,
			source_space_id: profile.source_space_id ?? '',
			source_space_name: '',
			target_space_ids: [...(profile.target_space_ids ?? [])],
			target_space_names: {},
			age: profile.demographics?.age !== undefined ? String(profile.demographics.age) : '',
			occupation: profile.demographics?.occupation ?? '',
			family_status: profile.demographics?.family_status ?? '',
			language_proficiency: profile.demographics?.language_proficiency ?? '',
			budget_min:
				profile.demographics?.budget_min !== undefined ? String(profile.demographics.budget_min) : '',
			budget_max:
				profile.demographics?.budget_max !== undefined ? String(profile.demographics.budget_max) : '',
			budget_currency: profile.demographics?.budget_currency ?? '',
			is_default: profile.is_default,
			goals: profile.goals ? profile.goals.map((g) => ({ ...g })) : [],
			preferences: profile.preferences
				? Object.entries(profile.preferences).map(([key, weight]) => ({ key, weight: Number(weight) || 0 }))
				: []
		};
	}

	function startNew() {
		loadDraft(null, true);
		view = 'edit';
	}

	function startEdit(profile: ComparisonProfile) {
		loadDraft(profile, false);
		view = 'edit';
	}

	async function callOnsave(profile: ComparisonProfile) {
		activeProfile = profile;
		try {
			await onsave?.(profile);
		} catch (e) {
			// Parent decides whether to surface this; drawer stays open on error.
			console.error('Profile onsave callback failed:', e);
		}
	}

	function selectProfile(profile: ComparisonProfile) {
		callOnsave(profile);
	}

	function addGoal() {
		draft.goals = [...draft.goals, { key: '', weight: 1 }];
	}

	function removeGoal(index: number) {
		draft.goals = draft.goals.filter((_, i) => i !== index);
	}

	function addPreference() {
		draft.preferences = [...draft.preferences, { key: '', weight: 1 }];
	}

	function removePreference(index: number) {
		draft.preferences = draft.preferences.filter((_, i) => i !== index);
	}

	async function save() {
		saving = true;
		error = '';
		try {
			const demographics: ComparisonProfileInput['demographics'] = {
				occupation: draft.occupation || undefined,
				family_status: draft.family_status || undefined,
				language_proficiency: draft.language_proficiency || undefined,
				budget_currency: draft.budget_currency || undefined
			};
			if (draft.age) {
				const parsed = parseInt(draft.age, 10);
				if (!isNaN(parsed)) demographics.age = parsed;
			}
			if (draft.budget_min) {
				const parsed = Number(draft.budget_min);
				if (!isNaN(parsed)) demographics.budget_min = parsed;
			}
			if (draft.budget_max) {
				const parsed = Number(draft.budget_max);
				if (!isNaN(parsed)) demographics.budget_max = parsed;
			}

			const input: ComparisonProfileInput = {
				title: draft.title,
				source_space_id: draft.source_space_id || null,
				target_space_ids: draft.target_space_ids,
				goals: draft.goals
					.filter((g) => g.key.trim())
					.map((g) => ({ key: g.key.trim(), weight: g.weight })),
				preferences: Object.fromEntries(
				draft.preferences
					.filter((p) => p.key.trim())
					.map((p) => [p.key.trim(), p.weight])
			),
				demographics,
				is_default: draft.is_default,
				is_template: false
			};

			const saved = draft.id ? await updateProfile(draft.id, input) : await createProfile(input);

			if (!saved) {
				throw new Error(t('jce.profile.save_error', 'Could not save profile'));
			}

			if (activeProfile?.id === saved.id || !draft.id) {
				activeProfile = saved;
			}

			await callOnsave(saved);
			view = 'select';
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}

	$effect.pre(() => {
		if (!open) {
			view = 'select';
		}
	});
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-40 bg-black/50" />
		<Dialog.Content
			class="fixed left-1/2 top-1/2 z-50 flex max-h-[85vh] w-[calc(100%-2rem)] max-w-2xl -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-[var(--radius-card)] border shadow-lg outline-none"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
		>
			<Dialog.Title class="sr-only">
				{t('jce.profile.title')}
			</Dialog.Title>
			<Dialog.Description class="sr-only">
				{t('jce.profile.select', 'Select or create a profile')}
			</Dialog.Description>

			{#if view === 'select'}
				<div class="flex items-center justify-between border-b p-4" style="border-color: var(--color-border)">
					<h2 class="flex items-center gap-2 text-base font-semibold" style="color: var(--color-text)">
						<User size={18} />
						{t('jce.profile.title')}
					</h2>
					<div class="flex items-center gap-2">
						<button
							type="button"
							onclick={startNew}
							class="inline-flex items-center gap-1 rounded-[var(--radius-button)] px-3 py-1.5 text-xs font-medium"
							style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
						>
							<Plus size={14} />
							{t('jce.profile.create')}
						</button>
						<Dialog.Close
							class="grid h-7 w-7 place-items-center rounded-[var(--radius-button)] transition-colors"
							style="background-color: var(--color-bg); color: var(--color-muted)"
							aria-label={t('jce.profile.cancel', 'Cancel')}
						>
							<X size={16} />
						</Dialog.Close>
					</div>
				</div>

				<div class="flex-1 overflow-y-auto p-4">
					{#if profileStats.length > 0}
						<div class="mb-4 rounded-[var(--radius-button)] border p-3" style="border-color: var(--color-border); background-color: var(--color-surface)">
							<h3 class="text-sm font-medium" style="color: var(--color-text)">
								{t('jce.profile.stats_heading', 'Profile usage')}
							</h3>
							<ul class="mt-2 space-y-1">
								{#each profileStats as stat}
									<li class="flex items-center justify-between text-xs" style="color: var(--color-muted)">
										<span class="truncate">
											{stat.source_space_name || stat.source_space_slug || stat.source_space_id}
										</span>
										<span class="font-medium" style="color: var(--color-text)">
											{stat.profile_count}
										</span>
									</li>
								{/each}
							</ul>
						</div>
					{/if}
					{#if profiles.length === 0}
						<p class="text-sm" style="color: var(--color-muted)">
							{t('jce.profile.no_profiles', 'No profiles yet. Create one to start.')}
						</p>
					{:else}
						<div class="space-y-2">
							{#each profiles as p (p.id)}
								<div
									class="flex items-center gap-3 rounded-[var(--radius-button)] border p-3 transition-colors"
									style="border-color: {activeProfile?.id === p.id ? 'var(--color-text)' : 'var(--color-border)'}; background-color: {activeProfile?.id === p.id ? 'var(--color-bg)' : 'var(--color-surface)'}"
								>
									<button
										type="button"
										onclick={() => selectProfile(p)}
										class="flex min-w-0 flex-1 flex-col items-start gap-1 text-left"
										style="color: var(--color-text)"
									>
										<span class="flex items-center gap-2 text-sm font-medium">
											{#if activeProfile?.id === p.id}
												<Check size={14} style="color: var(--color-text)" />
											{/if}
											{p.title}
										</span>
										{#if p.demographics}
											<span class="truncate text-xs" style="color: var(--color-muted)">
												{(p.target_space_ids ?? []).join(', ') || t('jce.profile.target_spaces')}
											</span>
										{/if}
									</button>
									<button
										type="button"
										onclick={() => startEdit(p)}
										class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius-button)]"
										style="background-color: var(--color-bg); color: var(--color-muted)"
										aria-label={t('jce.profile.edit')}
									>
										<Pencil size={14} />
									</button>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{:else}
				<div class="flex items-center justify-between border-b p-4" style="border-color: var(--color-border)">
					<h2 class="text-base font-semibold" style="color: var(--color-text)">
						{draft.id ? t('jce.profile.edit') : t('jce.profile.create')}
					</h2>
					<button
						type="button"
						onclick={() => (view = 'select')}
						class="text-xs font-medium"
						style="color: var(--color-muted)"
					>
						{t('jce.profile.back', 'Back')}
					</button>
				</div>

				<form
					class="flex-1 overflow-y-auto p-4"
					onsubmit={(e) => {
						e.preventDefault();
						save();
					}}
				>
					{#if error}
						<div
							class="mb-4 rounded-[var(--radius-button)] border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300"
						>
							{error}
						</div>
					{/if}

					<div class="space-y-4">
						<label class="block space-y-1">
							<span class="text-sm font-medium" style="color: var(--color-text)">
								{t('jce.profile.title_label')}
							</span>
							<input
								type="text"
								bind:value={draft.title}
								required
								class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
								style="border-color: var(--color-border); color: var(--color-text)"
							/>
						</label>

						<div class="grid gap-4 sm:grid-cols-2">
							<div class="relative block space-y-1">
								<span class="text-sm font-medium" style="color: var(--color-text)">
									{t('jce.profile.source_space')}
								</span>
								{#if draft.source_space_id}
									<div class="flex items-center gap-2 rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm" style="border-color: var(--color-border); color: var(--color-text)">
										<span class="flex-1 truncate">{draft.source_space_name || draft.source_space_id}</span>
										<button type="button" onclick={clearSource} class="text-[var(--color-muted)] hover:text-[var(--color-text)]">
											<X size={14} />
										</button>
									</div>
								{:else}
									<input
										type="text"
										value={sourceSearch}
										oninput={(e) => onSourceInput(e.currentTarget.value)}
										placeholder={t('jce.profile.source_space_placeholder', 'Search for a space…')}
										class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
										style="border-color: var(--color-border); color: var(--color-text)"
									/>
								{/if}
								{#if sourceSuggestions.length > 0}
									<div class="absolute z-10 mt-1 max-h-40 w-full overflow-y-auto rounded-[var(--radius-button)] border bg-[var(--color-surface)] shadow-md" style="border-color: var(--color-border)">
										{#each sourceSuggestions as s (s.id)}
											<button
												type="button"
												onclick={() => selectSource(s)}
												class="w-full px-3 py-2 text-left text-sm hover:bg-[var(--color-bg)]"
												style="color: var(--color-text)"
											>
												{s.name}
											</button>
											{/each}
										</div>
									{/if}
								</div>

							<div class="relative block space-y-1">
								<span class="text-sm font-medium" style="color: var(--color-text)">
									{t('jce.profile.target_spaces')}
								</span>
								{#if draft.target_space_ids.length > 0}
									<div class="mb-2 flex flex-wrap gap-1">
										{#each draft.target_space_ids as tid (tid)}
											<span class="inline-flex items-center gap-1 rounded-[var(--radius-button)] bg-[var(--color-bg)] px-2 py-1 text-xs" style="color: var(--color-text)">
												{draft.target_space_names[tid] || tid}
												<button type="button" onclick={() => removeTarget(tid)} class="text-[var(--color-muted)] hover:text-[var(--color-text)]">
													<X size={12} />
												</button>
											</span>
											{/each}
										</div>
									{/if}
									<input
										type="text"
										value={targetSearch}
										oninput={(e) => onTargetInput(e.currentTarget.value)}
										placeholder={t('jce.profile.target_spaces_placeholder', 'Search and add spaces…')}
										class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
										style="border-color: var(--color-border); color: var(--color-text)"
									/>
									{#if targetSuggestions.length > 0}
										<div class="absolute z-10 mt-1 max-h-40 w-full overflow-y-auto rounded-[var(--radius-button)] border bg-[var(--color-surface)] shadow-md" style="border-color: var(--color-border)">
											{#each targetSuggestions as s (s.id)}
												<button
													type="button"
													onclick={() => addTarget(s)}
													class="w-full px-3 py-2 text-left text-sm hover:bg-[var(--color-bg)]"
													style="color: var(--color-text)"
												>
													{s.name}
												</button>
												{/each}
											</div>
										{/if}
									</div>
								</div>

						<div>
							<h3 class="mb-2 text-sm font-medium" style="color: var(--color-text)">
								{t('jce.profile.goals')}
							</h3>
							<div class="space-y-2">
								{#each draft.goals as goal, i (i)}
									<div class="flex items-center gap-2">
										<JceSelect
											options={[
												{ value: '', label: t('jce.profile.goal_select', 'Select a goal') },
												...allowedGoals.map((g) => ({ value: g.key, label: g.label ?? g.key }))
											]}
											value={goal.key}
											onchange={(value) => (goal.key = value)}
											class="flex-1"
										/>
										<input
											type="number"
											bind:value={goal.weight}
											min="0"
											step="0.1"
											class="w-20 rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
											style="border-color: var(--color-border); color: var(--color-text)"
										/>
										<button
											type="button"
											onclick={() => removeGoal(i)}
											class="grid h-8 w-8 place-items-center rounded-[var(--radius-button)]"
											style="background-color: var(--color-bg); color: var(--color-muted)"
											aria-label={t('jce.common.delete')}
										>
											<X size={14} />
										</button>
									</div>
								{/each}
								<button
									type="button"
									onclick={addGoal}
									class="inline-flex items-center gap-1 text-xs font-medium"
									style="color: var(--color-text)"
								>
									<Plus size={14} />
									{t('jce.profile.add_goal')}
								</button>
							</div>
						</div>

						{#if scoreFactors.length > 0}
							<div>
								<h3 class="mb-2 text-sm font-medium" style="color: var(--color-text)">
									{t('jce.profile.preferences')}
								</h3>
								<div class="space-y-2">
									{#each draft.preferences as pref, i (i)}
										<div class="flex items-center gap-2">
											<JceSelect
												options={[
													{ value: '', label: t('jce.profile.preference_select', 'Select a factor') },
													...scoreFactors.map((f) => ({ value: f.key, label: f.label ?? f.key }))
												]}
												value={pref.key}
												onchange={(value) => (pref.key = value)}
												class="flex-1"
											/>
											<input
												type="number"
												bind:value={pref.weight}
												min="0"
												step="0.05"
												class="w-20 rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
												style="border-color: var(--color-border); color: var(--color-text)"
											/>
											<button
												type="button"
												onclick={() => removePreference(i)}
												class="grid h-8 w-8 place-items-center rounded-[var(--radius-button)]"
												style="background-color: var(--color-bg); color: var(--color-muted)"
												aria-label={t('jce.common.delete')}
											>
												<X size={14} />
											</button>
										</div>
									{/each}
									<button
										type="button"
										onclick={addPreference}
										class="inline-flex items-center gap-1 text-xs font-medium"
										style="color: var(--color-text)"
									>
										<Plus size={14} />
										{t('jce.profile.add_preference')}
									</button>
								</div>
							</div>
						{/if}

						<div>
							<h3 class="mb-2 text-sm font-medium" style="color: var(--color-text)">
								{t('jce.profile.about')}
							</h3>
							<div class="grid gap-4 sm:grid-cols-2">
								<label class="block space-y-1">
									<span class="text-xs" style="color: var(--color-muted)">
										{t('jce.profile.demographics.age')}
									</span>
									<input
										type="text"
										bind:value={draft.age}
										class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
										style="border-color: var(--color-border); color: var(--color-text)"
									/>
								</label>
								<label class="block space-y-1">
									<span class="text-xs" style="color: var(--color-muted)">
										{t('jce.profile.demographics.occupation')}
									</span>
									<input
										type="text"
										bind:value={draft.occupation}
										class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
										style="border-color: var(--color-border); color: var(--color-text)"
									/>
								</label>
								<label class="block space-y-1">
									<span class="text-xs" style="color: var(--color-muted)">
										{t('jce.profile.demographics.family_status')}
									</span>
									<input
										type="text"
										bind:value={draft.family_status}
										class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
										style="border-color: var(--color-border); color: var(--color-text)"
									/>
								</label>
								<label class="block space-y-1">
									<span class="text-xs" style="color: var(--color-muted)">
										{t('jce.profile.demographics.language_proficiency')}
									</span>
									<input
										type="text"
										bind:value={draft.language_proficiency}
										class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
										style="border-color: var(--color-border); color: var(--color-text)"
									/>
								</label>
								<label class="block space-y-1">
									<span class="text-xs" style="color: var(--color-muted)">
										{t('jce.profile.demographics.budget_min')}
									</span>
									<input
										type="text"
										bind:value={draft.budget_min}
										class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
										style="border-color: var(--color-border); color: var(--color-text)"
									/>
								</label>
								<label class="block space-y-1">
									<span class="text-xs" style="color: var(--color-muted)">
										{t('jce.profile.demographics.budget_max')}
									</span>
									<input
										type="text"
										bind:value={draft.budget_max}
										class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
										style="border-color: var(--color-border); color: var(--color-text)"
									/>
								</label>
								<label class="block space-y-1 sm:col-span-2">
									<span class="text-xs" style="color: var(--color-muted)">
										{t('jce.profile.demographics.budget_currency')}
									</span>
									<input
										type="text"
										bind:value={draft.budget_currency}
										class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
										style="border-color: var(--color-border); color: var(--color-text)"
									/>
								</label>
							</div>
						</div>

						<label class="flex items-center gap-2 text-sm" style="color: var(--color-text)">
							<input
								type="checkbox"
								bind:checked={draft.is_default}
								class="h-4 w-4 rounded border"
								style="border-color: var(--color-border)"
							/>
							{t('jce.profile.is_default')}
							{#if draft.is_default}
								<span class="text-xs" style="color: var(--color-muted)">({t('jce.profile.default')})</span>
							{/if}
						</label>
					</div>

					<div class="mt-6 flex items-center justify-end gap-2">
						<button
							type="button"
							onclick={() => (view = 'select')}
							class="rounded-[var(--radius-button)] border px-4 py-2 text-sm font-medium"
							style="border-color: var(--color-border); color: var(--color-text); background-color: var(--color-bg)"
						>
							{t('jce.profile.cancel')}
						</button>
						<button
							type="submit"
							disabled={saving || !draft.title.trim()}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius-button)] px-4 py-2 text-sm font-medium disabled:opacity-50"
							style="background-color: var(--color-primary); color: var(--color-primary-contrast)"
						>
							<Save size={14} />
							{saving ? t('jce.common.loading') : t('jce.profile.save')}
						</button>
					</div>
				</form>
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
