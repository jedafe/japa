<script lang="ts">
	// Ref: japaconnect-engineering/sections/05-geo-first-space-system.md
	// Simplified member space creation modal. Members choose a flat list of
	// selectable space types (default: community), enter a name, and the
	// backend keyword-matches the name against primary geo spaces to auto-link.
	import { Dialog } from 'bits-ui';
	import { X, Rocket, Loader2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { suggestSlug } from '../create/slug';
	import SpaceCombobox from './SpaceCombobox.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	interface SpaceType {
		id: string;
		name: string;
		slug: string;
	}

	interface SpaceListItem {
		id: string;
		name: string;
		slug: string;
	}

	interface PermissionPreset {
		name: string;
		visibility: string;
		posting_perm: string;
		replying_perm: string;
		commenting_perm: string;
		joining_perm: string;
	}

	interface Props {
		open?: boolean;
		/** When provided, called with the created space instead of navigating to it. */
		onCreated?: (space: { id: string; slug: string; name: string }) => void;
	}
	let { open = $bindable(false), onCreated }: Props = $props();

	let name = $state('');
	let slug = $state('');
	let slugTouched = $state(false);
	let description = $state('');
	let selectedSpaceTypeId = $state('');
	let spaceTypes = $state<SpaceType[]>([]);

	let submitting = $state(false);
	let error = $state('');

	// Name auto-suggest state (to prevent duplicate spaces).
	let nameSuggestions = $state<SpaceListItem[]>([]);
	let nameSearchLoading = $state(false);
	let nameSearchTimer: ReturnType<typeof setTimeout> | null = null;
	let showNameSuggestions = $state(false);

	// Quota display (BUG-008).
	let ownedSpacesCount = $state<number | null>(null);

	// Permission presets (BUG-009).
	let presets = $state<PermissionPreset[]>([]);
	let selectedPresetName = $state('');

	// Primary parent(s) for the new custom space (Section 05: custom spaces
	// must be linked to one or more primary spaces). We support multiple parents
	// via the parent_space_ids field.
	let selectedParents = $state<{ id: string; name: string; slug: string }[]>([]);

	$effect(() => {
		if (open) {
			fetchSpaceTypes();
			fetchOwnedSpacesCount();
			fetchPresets();
		}
	});

	async function fetchSpaceTypes() {
		try {
			const res = await api.get<SpaceType[]>('/space-types?selectable=true');
			if (res.data) {
				spaceTypes = res.data;
				if (!selectedSpaceTypeId) {
					const defaultSlug = (page.data.siteSettings?.['spaces.default_member_space_type_slug'] as string | undefined) ?? 'community';
					const defaultType = spaceTypes.find((st) => st.slug === defaultSlug);
					selectedSpaceTypeId = defaultType?.id ?? spaceTypes[0]?.id ?? '';
				}
			}
		} catch {
			spaceTypes = [];
		}
	}

	async function fetchOwnedSpacesCount() {
		try {
			const res = await api.get<{ id: string }[]>('/me/spaces/owned');
			ownedSpacesCount = res.data?.length ?? 0;
		} catch {
			ownedSpacesCount = null;
		}
	}

	async function fetchPresets() {
		try {
			const res = await api.get<PermissionPreset[]>('/space-permission-presets');
			presets = res.data ?? [];
		} catch {
			presets = [];
		}
	}

	// Name auto-suggest — search existing spaces to prevent duplicates.
	function searchNames(q: string) {
		if (nameSearchTimer) clearTimeout(nameSearchTimer);
		if (q.trim().length < 1) {
			nameSuggestions = [];
			showNameSuggestions = false;
			return;
		}
		nameSearchLoading = true;
		showNameSuggestions = true;
		nameSearchTimer = setTimeout(async () => {
			try {
				const res = await api.get<SpaceListItem[]>(`/spaces?q=${encodeURIComponent(q.trim())}`);
				nameSuggestions = res.data ?? [];
			} catch {
				nameSuggestions = [];
			} finally {
				nameSearchLoading = false;
			}
		}, 250);
	}

	function selectExistingName(space: SpaceListItem) {
		open = false;
		if (onCreated) {
			onCreated({ id: space.id, slug: space.slug, name: space.name });
		} else {
			goto(`/spaces/${space.slug}`);
		}
	}

	function onNameInput(e: Event) {
		name = (e.currentTarget as HTMLInputElement).value;
		if (!slugTouched) slug = suggestSlug(name);
		searchNames(name);
	}
	function onSlugInput(e: Event) {
		slug = (e.currentTarget as HTMLInputElement).value;
		slugTouched = true;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (!name.trim() || !slug.trim()) {
			error = t('spaces.create_error_fields');
			return;
		}
		if (!selectedSpaceTypeId) {
			error = t('spaces.no_types');
			return;
		}

		submitting = true;
		try {
			const body: Record<string, unknown> = {
				name: name.trim(),
				slug: slug.trim(),
				space_type_id: selectedSpaceTypeId,
				level: 'custom',
				description: description.trim() || undefined
			};
			if (selectedPresetName) body.preset_name = selectedPresetName;
			if (selectedParents.length > 0) body.parent_space_ids = selectedParents.map((p) => p.id);

			const res = await api.post<{ id: string; slug: string; name: string }>('/spaces', body);
			if (res.data?.slug) {
				open = false;
				if (onCreated && res.data.id) {
					onCreated(res.data);
				} else {
					await goto(`/spaces/${res.data.slug}`);
				}
			} else {
				// Surface the API error (duplicate slug, quota exceeded,
				// permission denied…) instead of a generic failure.
				error = res.error?.message ?? t('spaces.create_error');
			}
		} catch {
			error = t('spaces.create_error');
		} finally {
			submitting = false;
		}
	}

	function reset() {
		name = '';
		slug = '';
		slugTouched = false;
		description = '';
		selectedSpaceTypeId = '';
		selectedPresetName = '';
		selectedParents = [];
		nameSuggestions = [];
		showNameSuggestions = false;
		error = '';
	}
</script>

<Dialog.Root bind:open onOpenChange={(isOpen) => { if (!isOpen) reset(); }}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm" />
		<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-[560px] -translate-x-1/2 -translate-y-1/2 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-xl max-h-[90vh] overflow-y-auto">
			<Dialog.Close class="absolute right-4 top-4 text-[var(--color-muted)] hover:text-[var(--color-text)]">
				<X size={20} />
			</Dialog.Close>

			<div class="mb-4 flex items-center gap-2">
				<Rocket size={20} class="text-[var(--color-primary)]" />
				<h2 class="text-lg font-semibold text-[var(--color-text)]">{t('spaces.create_title')}</h2>
			</div>

			{#if spaceTypes.length === 0}
				<p class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-muted)]">
					{t('spaces.no_types')}
				</p>
			{:else}
				<form onsubmit={submit} class="space-y-3">
					<!-- Space type selector — flat dropdown of selectable member types -->
					<div>
						<label for="space-type" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
							{t('spaces.type_label', 'Space type')}
						</label>
						<AutocompleteSelect
							id="space-type"
							bind:value={selectedSpaceTypeId}
							options={spaceTypes.map((st) => ({ value: st.id, label: st.name }))}
							disabled={submitting}
						/>
					</div>

					<!-- Space name — auto-suggests existing spaces to prevent duplicates -->
					<div class="relative">
						<label for="space-name" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
							{t('spaces.name_label', 'Space name')}
						</label>
						<input
							id="space-name"
							value={name}
							oninput={onNameInput}
							onfocus={() => { if (name.trim()) showNameSuggestions = true; }}
							onblur={() => { setTimeout(() => (showNameSuggestions = false), 200); }}
							placeholder={t('spaces.name_community_placeholder', 'e.g. Frontend Devs in Lagos, Nigeria')}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
							disabled={submitting}
						/>
						{#if nameSearchLoading}
							<div class="absolute right-3 top-[26px]">
								<Loader2 size={14} class="animate-spin text-[var(--color-muted)]" />
							</div>
						{/if}
						{#if showNameSuggestions && nameSuggestions.length > 0}
							<div class="absolute z-20 mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-lg max-h-48 overflow-y-auto">
								{#each nameSuggestions as suggestion (suggestion.id)}
									<button
										type="button"
										class="flex w-full items-center justify-between px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
										onclick={() => selectExistingName(suggestion)}
									>
										<span>{suggestion.name}</span>
										<span class="text-xs text-[var(--color-muted)]">{t('spaces.exists_label', 'exists')}</span>
									</button>
								{/each}
							</div>
						{/if}
					</div>

					<div>
						<label for="space-slug" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('spaces.slug_label')}</label>
						<input
							id="space-slug"
							value={slug}
							oninput={onSlugInput}
							placeholder="my-space"
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
							disabled={submitting}
						/>
					</div>

					<div>
						<label for="space-desc" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('spaces.description_label')}</label>
						<textarea
							id="space-desc"
							bind:value={description}
							rows={2}
							placeholder={t('spaces.description_placeholder')}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
							disabled={submitting}
						></textarea>
					</div>

					<div>
						<p class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
							{t('spaces.parent_label', 'Primary parent space(s)')}
							<span class="font-normal text-[var(--color-muted)]">({t('spaces.optional_label', 'optional')})</span>
						</p>
						<SpaceCombobox
							parentMode
							selected={null}
							onSelect={(space) => {
								if (!selectedParents.find((p) => p.id === space.id)) {
									selectedParents = [...selectedParents, space];
								}
							}}
							placeholder={t('spaces.parent_placeholder', 'Search a country, state, or city…')}
							disabled={submitting}
						/>
						{#if selectedParents.length > 0}
							<div class="mt-2 flex flex-wrap gap-1.5">
								{#each selectedParents as parent (parent.id)}
									<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2 py-1 text-xs text-[var(--color-primary)]">
										{parent.name}
										<button
											type="button"
											onclick={() => { selectedParents = selectedParents.filter((p) => p.id !== parent.id); }}
											class="text-[var(--color-muted)] hover:text-[var(--color-text)]"
											aria-label={t('common.remove', 'Remove')}
										>
											<X size={11} />
										</button>
									</span>
								{/each}
							</div>
						{/if}
					</div>

					{#if presets.length > 0}
						<div>
							<label for="space-preset" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
								{t('spaces.preset_label', 'Permission preset')}
								<span class="font-normal text-[var(--color-muted)]">({t('spaces.optional_label', 'optional')})</span>
							</label>
							<AutocompleteSelect
								id="space-preset"
								bind:value={selectedPresetName}
								options={[
									{ value: '', label: t('spaces.preset_default', 'Default') },
									...presets.map((p) => ({ value: p.name, label: p.name }))
								]}
								disabled={submitting}
							/>
						</div>
					{/if}

					{#if ownedSpacesCount !== null}
						<p class="text-xs text-[var(--color-muted)]">
							{t('spaces.quota_used', 'Spaces created: {count}', { count: ownedSpacesCount })}
						</p>
					{/if}

					{#if error}
						<p class="text-sm text-red-500" role="alert">{error}</p>
					{/if}

					<div class="flex justify-end gap-2 pt-1">
						<button type="button" onclick={() => (open = false)} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">
							{t('spaces.create_cancel')}
						</button>
						<button type="submit" disabled={submitting} class="flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50">
							{#if submitting}
								<Loader2 size={14} class="animate-spin" />
								{t('spaces.creating', 'Creating…')}
							{:else}
								{t('spaces.create_submit')}
							{/if}
						</button>
					</div>
				</form>
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
