// Ref: japaconnect-engineering/sections/82-japa-planning-groups.md
<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { t, siteName} from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { untrack } from 'svelte';
	import { Users, ArrowLeft } from 'lucide-svelte';
	import type { PageData } from './$types';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';

	let { data }: { data: PageData } = $props();

	const group = $derived(data.group);

	let name = $state(untrack(() => group.name));
	let description = $state(untrack(() => group.description ?? ''));
	let destinationSpaceId = $state(untrack(() => group.destination_space_id));
	let visa_category = $state(untrack(() => group.visa_category ?? ''));
	let target_departure_window = $state(untrack(() => group.target_departure_window ?? ''));
	let max_members = $state(untrack(() => group.max_members ?? 15));
	let is_open_join = $state(untrack(() => group.is_open_join));
	let expiresAt = $state(untrack(() => group.expires_at ?? ''));
	let loading = $state(false);
	let errorKey = $state('');
	let errorMsg = $state('');

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]';

	async function submitEdit(e: SubmitEvent) {
		e.preventDefault();
		if (loading) return;
		loading = true;
		errorKey = '';
		errorMsg = '';
		try {
			const res = await api.patch(`/planning-groups/${group.id}`, {
				name: name.trim(),
				description: description.trim() || undefined,
				destination_space_id: destinationSpaceId,
				visa_category: visa_category.trim() || undefined,
				target_departure_window: target_departure_window.trim() || undefined,
				max_members: max_members,
				is_open_join: is_open_join,
				expires_at: expiresAt || undefined
			});
			if (res.data) {
				await goto(`/planning-groups/${group.id}`);
			} else {
				errorKey = res.error?.code ?? 'edit_failed';
				errorMsg = res.error?.message ?? t('planning_groups.errors.edit_failed', 'Failed to update group.');
			}
		} catch {
			errorMsg = t('planning_groups.errors.edit_failed', 'Failed to update group.');
		} finally {
			loading = false;
		}
	}

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('common.all_countries', 'All countries');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...(data.countries ?? []).map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = (data.countries ?? []).filter(
			(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
		);
		const items = filtered.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(matchesAll ? [allItem, ...items] : items);
	}
	function fetchDestinationCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinationCountries('');
	}
	const selectedDestinationLabel = $derived(
		destinationSpaceId ? (data.countries ?? []).find((c) => c.id === destinationSpaceId)?.name ?? null : null
	);
</script>

<svelte:head>
	<title>{t('planning_groups.edit_heading', 'Edit Planning Group')} — {siteName()}</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="mx-auto max-w-2xl px-4 py-6">
	<a
		href="/planning-groups/{group.id}"
		class="mb-4 inline-flex items-center gap-1 text-sm text-[var(--color-text-subtle)] hover:text-[var(--color-text)]"
	>
		<ArrowLeft class="rtl:-scale-x-100" size={14} /> {t('planning_groups.back_to_group', 'Back to group')}
	</a>
	<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
		<Users size={20} /> {t('planning_groups.edit_heading', 'Edit Planning Group')}
	</h1>
	<p class="mt-0.5 text-sm text-[var(--color-text-subtle)]">
		{t('planning_groups.edit_subtitle', 'Update your planning group details.')}
	</p>

	{#if errorMsg}
		<div
			class="mt-4 rounded-[var(--radius)] border border-[var(--color-danger)] bg-[var(--color-danger-subtle)] px-3 py-2 text-sm text-[var(--color-danger)]"
		>
			{errorMsg}
		</div>
	{/if}

	<form class="mt-6 space-y-4" onsubmit={submitEdit}>
		<div class="flex flex-col gap-1.5">
			<label for="pg-name" class="text-sm font-medium text-[var(--color-text)]">
				{t('planning_groups.field_name', 'Group name')}
			
			<HintIcon text={t('planning_groups.name_hint', 'A name others recognize — e.g. route + intake.')} /></label>
			<input id="pg-name" type="text" required bind:value={name} class={inputClass} maxlength="100" />
		</div>

		<AiBodyField
			id="pg-desc"
			label={t('planning_groups.field_description', 'Description')}
			contentType="planning_group"
			title={name}
			body={description}
			onBody={(v) => (description = v)}
			onTitle={(v) => (name = v)}
			user={data.user}
			features={data.features ?? {}}
			placeholder={t('planning_groups.field_description_placeholder', 'What is this group about?')}
			rows={3}
		/>

		<div class="flex flex-col gap-1.5">
			<label for="pg-dest" class="text-sm font-medium text-[var(--color-text)]">
				{t('planning_groups.field_destination', 'Destination country')}
			</label>
			<Autocomplete
				placeholder={"—"}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchDestinationCountries}
				initialFetcher={fetchDestinationCountriesInitial}
				selectedId={destinationSpaceId || null}
				selectedLabel={selectedDestinationLabel}
				onSelect={(item) => (destinationSpaceId = item.id)}
				onClear={() => (destinationSpaceId = '')}
			/>
			<HintIcon text={t('planning_groups.destination_hint', 'The destination country this group is planning fovisa" class="text-sm font-medium text-[var(--color-text)]">
					{t('planning_groups.field_visa_category', 'Visa category (optional)')}
				</label>
				<input id="pg-visa" type="text" bind:value={visa_category} class={inputClass} maxlength="50" />
				<HintIcon text={t('planning_groups.visa_hint', 'The visa type or route — e.g. Student, Work.')} />
			</div>
			<div class="flex flex-col gap-1.5">
				<label for="pg-window" class="text-sm font-medium text-[var(--color-text)]">
					{t('planning_groups.field_departure_window', 'Departure window (optional)')}
				</label>
				<input id="pg-window" type="text" bind:value={target_departure_window} class={inputClass} maxlength="50" />
				<HintIcon text={t('planning_groups.window_hint', 'The intake or window you are targeting — e.g. Q3 2026.')} />
			</div>
		</div>

		<div class="grid grid-cols-2 gap-4">
			<div class="flex flex-col gap-1.5">
				<label for="pg-max" class="text-sm font-medium text-[var(--color-text)]">
					{t('planning_groups.field_max_members', 'Max members')}
				</label>
				<input
					id="pg-max"
					type="number"
					min="2"
					max="100"
					value={max_members}
					oninput={(e) => (max_members = Number(e.currentTarget.value) || 15)}
					class={inputClass}
				/>
					<HintIcon text={t('planning_groups.max_hint', 'Optional — cap on group size.')} />
			</div>
			<div class="flex flex-col gap-1.5">
				<label for="pg-expires" class="text-sm font-medium text-[var(--color-text)]">
					{t('planning_groups.field_expires_at', 'Expiry date (optional)')}
				</label>
				<input id="pg-expires" type="date" bind:value={expiresAt} class={inputClass} />
				<HintIcon text={t('planning_groups.expires_hint', 'Optional — when the group closes to new members.')} />
			</div>
		</div>

		<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
			<input type="checkbox" bind:checked={is_open_join} class="rounded" />
			{t('planning_groups.field_open_join', 'Allow anyone to join (open join)')}
		</label>

		<div class="flex justify-end gap-2 pt-2">
			<a
				href="/planning-groups/{group.id}"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('common.cancel', 'Cancel')}
			</a>
			<button
				type="submit"
				disabled={loading}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				{loading ? '...' : t('planning_groups.save', 'Save changes')}
			</button>
		</div>
	</form>
</div>
