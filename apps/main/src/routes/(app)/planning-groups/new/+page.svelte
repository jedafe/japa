<script lang="ts">
	// Ref: japaconnect-engineering/sections/82-japa-planning-groups.md
	// Create planning group form. Submits via the browser API client (POST
	// /planning-groups), then redirects to the new group's detail page.
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { Users, Plus, ArrowLeft } from 'lucide-svelte';
	import type { PageData } from './$types';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';

	let { data }: { data: PageData } = $props();

	let name = $state('');
	let description = $state('');
	let destinationSpaceId = $state('');
	let visa_category = $state('');
	let target_departure_window = $state('');
	let max_members = $state(15);
	let is_open_join = $state(true);
	let loading = $state(false);
	let errorKey = $state('');

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!name.trim()) {
			errorKey = 'planning_groups.errors.name_required';
			return;
		}
		if (!destinationSpaceId) {
			errorKey = 'planning_groups.errors.destination_required';
			return;
		}
		loading = true;
		try {
			const res = await api.post<{ id: string }>('/planning-groups', {
				name: name.trim(),
				description: description.trim() || undefined,
				destination_space_id: destinationSpaceId,
				visa_category: visa_category.trim() || undefined,
				target_departure_window: target_departure_window.trim() || undefined,
				max_members: max_members,
				is_open_join: is_open_join
			});
			if (res.data) {
				await goto(`/planning-groups/${res.data.id}`);
				return;
			}
			errorKey = 'planning_groups.errors.create_failed';
		} catch {
			errorKey = 'planning_groups.errors.create_failed';
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
	<title>{t('planning_groups.new_title', 'Create Planning Group — JapaConnect')}</title>
	<meta
		name="description"
		content={t('planning_groups.new_description', 'Create a planning group to organize your japa journey with others.')}
	/>
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/planning-groups"
			class="mb-2 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={14} /> {t('planning_groups.back', 'All planning groups')}
		</a>
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Users size={20} /> {t('planning_groups.new_heading', 'Create Planning Group')}
		</h1>
		<p class="mt-0.5 text-sm text-[var(--color-text-subtle)]">
			{t('planning_groups.new_subtitle', 'Set up a group to plan your japa journey together.')}
		</p>
	</div>

	<div class="p-4">
		<form
			onsubmit={submit}
			class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5"
			novalidate
		>
			<div class="flex flex-col gap-1.5">
				<label for="pg-name" class="text-sm font-medium text-[var(--color-text)]">
					{t('planning_groups.field_name', 'Group name')} <span class="text-red-500">*</span>
				</label>
				<input
					id="pg-name"
					type="text"
					value={name}
					oninput={(e) => (name = e.currentTarget.value)}
					placeholder={t('planning_groups.field_name_placeholder', 'e.g. Canada Student Visa Q3 2026')}
					class={inputClass}
				/>
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
				rows={4}
			/>

			<div class="grid gap-4 sm:grid-cols-2">
				<div class="flex flex-col gap-1.5">
					<label for="pg-dest" class="text-sm font-medium text-[var(--color-text)]">
						{t('planning_groups.field_destination', 'Destination country')} <span class="text-red-500">*</span>
					</label>
					<Autocomplete
						placeholder={t('planning_groups.field_destination_placeholder', 'e.g. Canada')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchDestinationCountries}
						initialFetcher={fetchDestinationCountriesInitial}
						selectedId={destinationSpaceId || null}
						selectedLabel={selectedDestinationLabel}
						onSelect={(item) => (destinationSpaceId = item.id)}
						onClear={() => (destinationSpaceId = '')}
					/>
				</div>

				<div class="flex flex-col gap-1.5">
					<label for="pg-visa" class="text-sm font-medium text-[var(--color-text)]">
						{t('planning_groups.field_visa', 'Visa category')}
					</label>
					<input
						id="pg-visa"
						type="text"
						value={visa_category}
						oninput={(e) => (visa_category = e.currentTarget.value)}
						placeholder={t('planning_groups.field_visa_placeholder', 'e.g. Student')}
						class={inputClass}
					/>
				</div>
			</div>

			<div class="grid gap-4 sm:grid-cols-2">
				<div class="flex flex-col gap-1.5">
					<label for="pg-window" class="text-sm font-medium text-[var(--color-text)]">
						{t('planning_groups.field_window', 'Target departure window')}
					</label>
					<input
						id="pg-window"
						type="text"
						value={target_departure_window}
						oninput={(e) => (target_departure_window = e.currentTarget.value)}
						placeholder={t('planning_groups.field_window_placeholder', 'e.g. Q3 2026')}
						class={inputClass}
					/>
				</div>

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
				</div>
			</div>

			<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
				<input
					type="checkbox"
					checked={is_open_join}
					onchange={(e) => (is_open_join = e.currentTarget.checked)}
					class="h-4 w-4 rounded border-[var(--color-border)]"
				/>
				{t('planning_groups.field_open_join', 'Allow anyone to join (open join)')}
			</label>

			{#if errorKey}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{t(errorKey, 'Failed to create group.')}
				</p>
			{/if}

			<div class="flex justify-end gap-2 border-t border-[var(--color-border)] pt-4">
				<a
					href="/planning-groups"
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					{t('common.cancel', 'Cancel')}
				</a>
				<button
					type="submit"
					disabled={loading}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					<Plus size={16} />
					{loading ? t('common.loading', 'Loading…') : t('planning_groups.create', 'Create Group')}
				</button>
			</div>
		</form>
	</div>
</div>
