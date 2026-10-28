<script lang="ts">
	// Ref: japaconnect-engineering/sections/38-tickets.md (user portal)
	// Full-page new-ticket form. Submits via the browser API client (POST /tickets),
	// then redirects to the new ticket's thread view. The type selector is a
	// Bits UI Select populated from GET /ticket-types?is_public=true.
	import { t } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { ticketHref } from '$lib/tickets';
	import type { TicketPriority, TicketType } from '$lib/types';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	let { data } = $props();
	const types = $derived(data.types as TicketType[]);

	let typeId = $state('');
	let subject = $state('');
	let description = $state('');
	let priority = $state<TicketPriority>('normal');
	let email = $state('');
	let loading = $state(false);
	let errorKey = $state('');

	const priorities: { value: TicketPriority; key: string }[] = [
		{ value: 'low', key: 'tickets.priority.low' },
		{ value: 'normal', key: 'tickets.priority.normal' },
		{ value: 'high', key: 'tickets.priority.high' },
		{ value: 'urgent', key: 'tickets.priority.urgent' }
	];

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!subject.trim()) {
			errorKey = 'tickets.errors.subject_required';
			return;
		}
		if (!description.trim()) {
			errorKey = 'tickets.errors.description_required';
			return;
		}
		loading = true;
		try {
			const res = await api.post<{ id: string; ticket_number?: number }>('/tickets', {
				type_id: typeId || undefined,
				subject: subject.trim(),
				description: description.trim(),
				priority,
				email: email.trim() || undefined
			});
			if (res.data) {
				await goto(ticketHref(res.data));
				return;
			}
			errorKey = 'tickets.errors.create_failed';
		} catch {
			errorKey = 'tickets.errors.create_failed';
		} finally {
			loading = false;
		}
	}

	// Autocomplete fetchers — filter the already-loaded ticket types client-side.
	function fetchTypes(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? types
			: types.filter((tp) => tp.name.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((tp) => ({ id: tp.id, label: tp.name })));
	}
	function fetchTypesInitial(): Promise<AutocompleteItem[]> {
		return fetchTypes('');
	}
	const selectedTypeLabel = $derived(
		typeId ? types.find((tp) => tp.id === typeId)?.name ?? null : null
	);
</script>

<section class="space-y-4">
	<header>
		<a href="/dashboard/tickets" class="text-sm text-[var(--color-muted)] hover:underline">
			← {t('tickets.back_to_list')}
		</a>
		<h1 class="mt-2 text-lg font-medium text-[var(--color-text)]">{t('tickets.new_title')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('tickets.new_subtitle')}</p>
	</header>

	{#if errorKey}
		<p class="rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">
			{t(errorKey)}
		</p>
	{/if}

	<form onsubmit={submit} class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5" novalidate>
		{#if types.length > 0}
			<div class="flex flex-col gap-1.5">
				<label for="ticket-type" class="text-sm font-medium text-[var(--color-text)]">
					{t('tickets.field_type')}
				</label>
				<Autocomplete
					placeholder={t('tickets.type_placeholder')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchTypes}
					initialFetcher={fetchTypesInitial}
					selectedId={typeId || null}
					selectedLabel={selectedTypeLabel}
					onSelect={(item) => {
						typeId = item.id;
						const found = types.find((tp) => tp.id === item.id);
						if (found?.default_priority) priority = found.default_priority;
					}}
					onClear={() => {
						typeId = '';
					}}
				/>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('tickets.type_hint', 'What kind of help you need — support, billing, bug.')}</p>
			</div>
		{/if}

		<div class="flex flex-col gap-1.5">
			<label for="ticket-subject" class="text-sm font-medium text-[var(--color-text)]">
				{t('tickets.field_subject')}
			</label>
			<input
				id="ticket-subject"
				value={subject}
				oninput={(e) => (subject = e.currentTarget.value)}
				required
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
			/>
		</div>

		<div class="flex flex-col gap-1.5">
			<label for="ticket-description" class="text-sm font-medium text-[var(--color-text)]">
				{t('tickets.field_description')}
			</label>
			<textarea
				id="ticket-description"
				value={description}
				oninput={(e) => (description = e.currentTarget.value)}
				rows={8}
				required
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
			></textarea>
		</div>

		<div class="flex flex-col gap-1.5">
			<label for="ticket-priority" class="text-sm font-medium text-[var(--color-text)]">
				{t('tickets.field_priority')}
			</label>
			<AutocompleteSelect
				id="ticket-priority"
				value={priority}
				options={priorities.map((p) => ({ value: p.value, label: t(p.key) }))}
				ariaLabel={t('tickets.field_priority')}
				onValueChange={(v) => (priority = (v as TicketPriority) || 'normal')}
			/>
		</div>

		<div class="flex flex-col gap-1.5">
			<label for="ticket-email" class="text-sm font-medium text-[var(--color-text)]">
				{t('tickets.field_email')}
			</label>
			<input
				id="ticket-email"
				type="email"
				value={email}
				oninput={(e) => (email = e.currentTarget.value)}
				placeholder={t('tickets.field_email_optional')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
			/>
				<p class="mt-1 text-xs text-[var(--color-muted)]">{t('tickets.email_hint', 'Optional — a different contact email for replies.')}</p>
		</div>

		<div class="flex justify-end gap-2">
			<a
				href="/dashboard/tickets"
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				{t('tickets.cancel')}
			</a>
			<button
				type="submit"
				disabled={loading}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50"
			>
				{loading ? t('common.loading') : t('tickets.submit')}
			</button>
		</div>
	</form>
</section>
