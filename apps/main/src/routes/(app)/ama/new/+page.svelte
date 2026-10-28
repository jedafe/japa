<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ArrowLeft, Send } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';

	let { data }: { data: PageData } = $props();

	let title = $state('');
	let body = $state('');
	let hostProfession = $state('');
	let hostDestCountryId = $state('');
	let hostYearsAbroad = $state('');
	let openDurationHours = $state('48');
	let questionApprovalRequired = $state(false);

	let loading = $state(false);
	let errorKey = $state('');
	let errorCode = $state('');
	let activeSessionId = $state('');
	let cooldownUntil = $state('');

	const canSubmit = $derived(title.trim().length > 0 && hostDestCountryId.length > 0 && !loading);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none';

	function slugify(s: string): string {
		return s
			.toLowerCase()
			.trim()
			.replace(/[^a-z0-9\s-]/g, '')
			.replace(/\s+/g, '-')
			.replace(/-+/g, '-')
			.slice(0, 80);
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!canSubmit) return;
		loading = true;
		try {
			const bodyPayload: Record<string, unknown> = {
				title: title.trim(),
				slug: slugify(title) || `ama-${Date.now()}`,
				body: body ? JSON.stringify([{ type: 'paragraph', content: [{ type: 'text', text: body }] }]) : '[]',
				host_profession: hostProfession.trim(),
				host_dest_country_id: hostDestCountryId,
				open_duration_hours: Number(openDurationHours) || 48,
				question_approval_required: questionApprovalRequired
			};
			if (hostYearsAbroad.trim()) {
				const yrs = Number(hostYearsAbroad);
				if (!Number.isNaN(yrs)) bodyPayload.host_years_abroad = yrs;
			}

			const res = await api.post<{ id: string }>('/forum/ama', bodyPayload);
			if (res.data?.id) {
				loading = false;
				await goto(`/ama/${res.data.id}`);
			} else if (res.error) {
				errorCode = res.error.code ?? '';
				if (errorCode === 'ama_active_exists') {
					errorKey = 'ama.errors.active_exists';
					activeSessionId =
						((res.error.data as { session_id?: string } | undefined)?.session_id ?? '');
				} else if (errorCode === 'ama_cooldown') {
					errorKey = 'ama.errors.cooldown';
					cooldownUntil =
						((res.error.data as { retry_after?: string } | undefined)?.retry_after ?? '');
				} else {
					errorKey = 'ama.errors.submit_failed';
				}
				loading = false;
			} else {
				errorKey = 'ama.errors.submit_failed';
				loading = false;
			}
		} catch {
			errorKey = 'ama.errors.submit_failed';
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
		hostDestCountryId ? (data.countries ?? []).find((c) => c.id === hostDestCountryId)?.name ?? null : null
	);
</script>

<svelte:head>
	<title>{t('ama.new_title', 'Start an AMA — {site}')}</title>
	<meta name="description" content={t('ama.new_description', 'Open yourself to community questions in a structured AMA session.')} />
	<meta name="robots" content="noindex" />
	<meta property="og:title" content={t('ama.new_title', 'Start an AMA')} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="mx-auto flex max-w-2xl items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/ama"
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('ama.back', 'Back to AMAs')}
		>
			<ArrowLeft class="rtl:-scale-x-100" size={18} />
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('ama.new_heading', 'Start an AMA')}
		</h1>
	</div>

	<form onsubmit={submit} class="mx-auto max-w-2xl space-y-5 p-4" novalidate>
		<!-- Destination country (geo-aware: first input per §§79–101) -->
		<div class="flex flex-col gap-1.5">
			<label for="host_dest_country_id" class="text-sm font-medium text-[var(--color-text)]">
				{t('ama.field_destination', 'Destination country')} <span class="text-red-500">*</span>
			</label>
			<HintIcon text={t('ama.destination_hint', 'The country your experience relates to.')} />
			<Autocomplete
				placeholder={t('ama.placeholder_destination', 'Select destination country')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchDestinationCountries}
				initialFetcher={fetchDestinationCountriesInitial}
				selectedId={hostDestCountryId || null}
				selectedLabel={selectedDestinationLabel}
				onSelect={(item) => (hostDestCountryId = item.id)}
				onClear={() => (hostDestCountryId = '')}
			/>
		</div>

		<!-- Title -->
		<div class="flex flex-col gap-1.5">
			<label for="title" class="text-sm font-medium text-[var(--color-text)]">
				{t('ama.field_title', 'AMA title')} <span class="text-red-500">*</span>
			</label>
			<HintIcon text={t('ama.title_hint', 'Format: Ask a [profession] who [did the thing].')} />
			<input
				id="title"
				type="text"
				value={title}
				oninput={(e) => (title = e.currentTarget.value)}
				placeholder={t('ama.placeholder_title', 'e.g. Ask a Nurse who relocated to the UK in 2021')}
				class={inputClass}
				required
			/>
		</div>

		<!-- Body / intro -->
		<AiBodyField
			id="body"
			label={t('ama.field_intro', 'Introduction')}
			contentType="ama"
			{title}
			body={body}
			onBody={(v) => (body = v)}
			onTitle={(v) => (title = v)}
			user={data.user}
			features={data.features ?? {}}
			placeholder={t('ama.placeholder_intro', 'Introduce yourself and the kind of questions you can answer…')}
			rows={5}
		/>
			<HintIcon text={t('ama.intro_hint', 'Your background and the topics you can answer — builds trust.')} />

		<!-- Host profession + years abroad -->
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
			<div class="flex flex-col gap-1.5">
				<label for="host_profession" class="text-sm font-medium text-[var(--color-text)]">
					{t('ama.field_profession', 'Profession')}
				</label>
				<HintIcon text={t('ama.profession_hint', 'Your profession — helps users know who they are asking.')} />
				<input
					id="host_profession"
					type="text"
					value={hostProfession}
					oninput={(e) => (hostProfession = e.currentTarget.value)}
					placeholder={t('ama.placeholder_profession', 'e.g. Nurse')}
					class={inputClass}
				/>
			</div>
			<div class="flex flex-col gap-1.5">
				<label for="host_years_abroad" class="text-sm font-medium text-[var(--color-text)]">
					{t('ama.field_years_abroad', 'Years abroad')}
				</label>
				<HintIcon text={t('ama.years_hint', 'How many years you have lived abroad.')} />
				<input
					id="host_years_abroad"
					type="number"
					min="0"
					value={hostYearsAbroad}
					oninput={(e) => (hostYearsAbroad = e.currentTarget.value)}
					placeholder={t('ama.placeholder_years_abroad', 'e.g. 3')}
					class={inputClass}
				/>
			</div>
		</div>

		<!-- Open duration -->
		<div class="flex flex-col gap-1.5">
			<label for="open_duration_hours" class="text-sm font-medium text-[var(--color-text)]">
				{t('ama.field_duration', 'Session duration (hours)')}
			</label>
			<HintIcon text={t('ama.duration_hint', 'How long the AMA stays open for questions, in hours.')} />
			<input
				id="open_duration_hours"
				type="number"
				min="1"
				max="168"
				value={openDurationHours}
				oninput={(e) => (openDurationHours = e.currentTarget.value)}
				class={inputClass}
			/>
		</div>

		<!-- Question approval -->
		<label class="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-text-subtle)]">
			<input
				type="checkbox"
				checked={questionApprovalRequired}
				onchange={(e) => (questionApprovalRequired = e.currentTarget.checked)}
				class="h-4 w-4 accent-[var(--color-primary)]"
			/>
			{t('ama.field_question_approval', 'Require approval before questions appear')}
		</label>

		{#if errorKey}
			<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				{t(errorKey, 'Failed to start your AMA. Please try again.')}
			</p>
		{/if}

		{#if errorCode === 'ama_active_exists' && activeSessionId}
			<a
				href={`/ama/${activeSessionId}`}
				class="block rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm text-[var(--color-primary)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('ama.view_active_session', 'View your active session')}
			</a>
		{/if}

		{#if errorCode === 'ama_cooldown' && cooldownUntil}
			<p class="px-1 text-sm text-[var(--color-muted)]">
				{t('ama.retry_after', 'You can start a new session after:')}
				<time datetime={cooldownUntil}>{new Date(cooldownUntil).toLocaleString()}</time>
			</p>
		{/if}

		<!-- Submit -->
		<div class="flex items-center justify-end gap-2 border-t border-[var(--color-border)] pt-4">
			<a
				href="/ama"
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('common.cancel', 'Cancel')}
			</a>
			<button
				type="submit"
				disabled={!canSubmit}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				<Send class="rtl:-scale-x-100" size={16} /> {loading ? t('common.submitting', 'Submitting…') : t('ama.start', 'Start an AMA')}
			</button>
		</div>
	</form>
</div>
