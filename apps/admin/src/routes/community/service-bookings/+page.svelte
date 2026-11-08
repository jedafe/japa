<script lang="ts">
	// /community/service-bookings — admin bookings inbox + dispute-resolution
	// queue for §111 service packages.
	// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '#lib/components/entity/index.js';
	import { serviceBookingsAdminApi } from '#lib/api.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { hasPermission } from '#lib/stores/permissions.svelte.js';
	import { formatMoney } from '#lib/utils/currency.js';
	import { Eye, ArrowRightLeft } from 'lucide-svelte';
	import { Modal } from '#lib/components/ui/modal/index.js';
	import AutocompleteSelect from '#lib/components/ui/AutocompleteSelect.svelte';
	import type { ActionItem, FilterDef } from '#lib/components/ui/data-table/index.js';

	// Backend BookingDTO shape (§111). The admin list endpoint returns
	// { data: BookingItem[], meta: { page, limit, total } }.
	type BookingItem = {
		id: string;
		listing_title?: string;
		listing_slug?: string;
		listing_kind?: string;
		intent: string;
		status: string;
		party_size?: number;
		note?: string;
		preferred_dates?: { start_on?: string; end_on?: string; flexible?: boolean } | null;
		departure_starts_on?: string;
		departure_ends_on?: string;
		booker_username?: string;
		booker_display_name?: string;
		provider_username?: string;
		provider_display_name?: string;
		org_name?: string;
		org_slug?: string;
		destination_space_name?: string;
		tier_name?: string;
		coupon_code?: string;
		discount_amount?: number | null;
		purchase_mode?: string;
		payment_id?: string;
		payment_status?: string;
		payment_provider?: string;
		total_price?: number | null;
		fee_amount?: number | null;
		fee_currency?: string;
		agent_balance_due?: number | null;
		org_response_note?: string;
		dispute_reason?: string;
		cancel_reason?: string;
		expires_at?: string;
		confirmed_at?: string;
		fulfilled_at?: string;
		cancelled_at?: string;
		created_at: string;
	};

	const canAdmin = $derived(hasPermission('service.booking.admin'));

	// Mirrors the backend adminAllowedTransitions matrix (§111): terminal
	// rows (cancelled/declined/refunded) stay terminal, refunded is
	// reachable only out of disputed. The modal offers only legal targets
	// instead of every status.
	const ADMIN_TARGETS: Record<string, string[]> = {
		pending: ['confirmed', 'declined', 'cancelled'],
		confirmed: ['fulfilled', 'cancelled', 'disputed'],
		fulfilled: ['disputed'],
		disputed: ['refunded', 'confirmed', 'fulfilled', 'cancelled']
	};
	const OPEN_STATUSES = new Set(['pending', 'confirmed', 'fulfilled', 'disputed']);
	const ALL_STATUSES = ['pending', 'confirmed', 'declined', 'fulfilled', 'cancelled', 'disputed', 'refunded'];

	let entityController = $state<EntityPageController | undefined>(undefined);
	let acting = $state(false);

	// Transition modal state — target status + resolution note.
	let transitionTarget = $state<BookingItem | null>(null);
	let transitionStatus = $state('');
	let resolutionNote = $state('');
	let transitionError = $state('');

	function openTransition(row: BookingItem) {
		transitionTarget = row;
		transitionStatus = '';
		resolutionNote = '';
		transitionError = '';
	}

	async function submitTransition() {
		if (!transitionTarget || !transitionStatus) return;
		acting = true;
		transitionError = '';
		try {
			const result = await serviceBookingsAdminApi.transition(transitionTarget.id, {
				status: transitionStatus,
				resolution_note: resolutionNote.trim() || undefined
			});
			if (result.ok) {
				transitionTarget = null;
				entityController?.reload();
			} else {
				transitionError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			transitionError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: BookingItem, ctx) => ctx?.openView(row) }
		];
		if (canAdmin) {
			acts.push({
				label: t('app.service_bookings.set_status', 'Set status'),
				icon: ArrowRightLeft,
				onclick: openTransition,
				show: (row: BookingItem) => OPEN_STATUSES.has(row.status)
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'listing_title', label: t('app.service_bookings.col_listing', 'Listing'), type: 'text' },
		{ key: 'booker_username', label: t('app.service_bookings.col_booker', 'Booker'), type: 'text' },
		{ key: 'org_name', label: t('app.service_bookings.col_org', 'Organization'), type: 'text' },
		{ key: 'status', label: t('app.service_bookings.col_status', 'Status'), type: 'text' }
	];

	const filters = $derived.by<FilterDef[]>(() => [
		{
			key: 'status',
			label: t('app.service_bookings.col_status', 'Status'),
			options: ALL_STATUSES.map((s) => ({ value: s, label: t(`app.service_bookings.status_${s}`, s) }))
		},
		{
			key: 'intent',
			label: t('app.service_bookings.col_intent', 'Type'),
			options: [
				{ value: 'inquiry', label: t('app.service_bookings.intent_inquiry', 'Inquiry') },
				{ value: 'booking', label: t('app.service_bookings.intent_booking', 'Booking') }
			]
		},
		{
			key: 'kind',
			label: t('app.service_bookings.col_kind', 'Kind'),
			options: [
				{ value: 'service', label: t('app.service_bookings.kind_service', 'Service') },
				{ value: 'package', label: t('app.service_bookings.kind_package', 'Package') },
				{ value: 'mentorship', label: t('app.service_bookings.kind_mentorship', 'Mentorship') }
			]
		}
	]);

	const config = $derived<EntityPageConfig>({
		filters,
		title: t('app.service_bookings.heading', 'Service Bookings'),
		description: t('app.service_bookings.desc', 'Service bookings, inquiries, and dispute resolution'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'listing_title', label: t('app.service_bookings.col_listing', 'Listing'), sortable: true },
			{
				key: 'booker_username',
				label: t('app.service_bookings.col_booker', 'Booker'),
				sortable: true,
				render: (_v: string, row: BookingItem) =>
					row.booker_username
						? `@${row.booker_username}${row.booker_display_name ? ` · ${row.booker_display_name}` : ''}`
						: '—'
			},
			{
				key: 'org_name',
				label: t('app.service_bookings.col_org', 'Organization'),
				sortable: true
			},
			{
				key: 'intent',
				label: t('app.service_bookings.col_intent', 'Type'),
				sortable: true,
				render: (v: string) => t(`app.service_bookings.intent_${v}`, v)
			},
			{
				key: 'status',
				label: t('app.service_bookings.col_status', 'Status'),
				sortable: true,
				render: (v: string) => t(`app.service_bookings.status_${v}`, v)
			},
			{
				key: 'listing_kind',
				label: t('app.service_bookings.col_kind', 'Kind'),
				sortable: true,
				render: (v: string) => (v ? t(`app.service_bookings.kind_${v}`, v) : '—')
			},
			{
				key: 'total_price',
				label: t('app.service_bookings.col_total', 'Total'),
				render: (_v: number | null, row: BookingItem) =>
					row.total_price != null ? formatMoney(row.total_price, row.fee_currency, null, { minor: true }) : '—'
			},
			{
				key: 'created_at',
				label: t('app.service_bookings.col_created', 'Created'),
				sortable: true,
				render: (v: string) => (v ? new Date(v).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			const qs = new URLSearchParams();
			qs.set('page', String((params.page ?? 0) + 1));
			if (params.pageSize) qs.set('limit', String(params.pageSize));
			if (params.filter?.status) qs.set('status', params.filter.status);
			if (params.filter?.intent) qs.set('intent', params.filter.intent);
			if (params.filter?.kind) qs.set('kind', params.filter.kind);
			if (params.q) qs.set('q', params.q);
			if (params.sort) {
				// EntityPage passes sort as "key:dir" (e.g. "status:asc") —
				// map onto the backend sort vocabulary (latest|oldest|
				// status(_desc)|title|intent|kind|org|mentee_asc|mentee_desc).
				const [key, dir] = params.sort.split(':');
				const d = dir === 'asc' ? 'asc' : 'desc';
				const token = {
					listing_title: `title_${d}`,
					booker_username: `mentee_${d}`,
					org_name: `org_${d}`,
					intent: `intent_${d}`,
					status: d === 'asc' ? 'status' : 'status_desc',
					listing_kind: `kind_${d}`,
					created_at: d === 'asc' ? 'oldest' : 'latest'
				}[key];
				if (token) qs.set('sort', token);
			}
			const result = await serviceBookingsAdminApi.list(qs.toString());
			if (result.ok) {
				const data = (result.data?.data as BookingItem[]) ?? [];
				const total = result.data?.meta?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / (params.pageSize || 25)));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: BookingView
	});

	function fmtDate(iso?: string): string {
		return iso ? new Date(iso).toLocaleString() : '—';
	}
</script>

<svelte:head>
	<title>{t('app.service_bookings.page_title', 'Service Bookings — {site} Admin')}</title>
</svelte:head>

<EntityPage {config} bind:controller={entityController} />

<!-- ── Set-status / dispute-resolution modal ─────────────────────────────── -->
<Modal
	open={transitionTarget !== null}
	onclose={() => (transitionTarget = null)}
	title={t('app.service_bookings.transition_title', 'Set booking status')}
>
	{#if transitionTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{transitionTarget.listing_title || '—'} ·
			{t(`app.service_bookings.status_${transitionTarget.status}`, transitionTarget.status)}
			→
		</p>
		{#if transitionTarget.status === 'disputed' && transitionTarget.dispute_reason}
			<p class="mt-2 rounded-lg p-2 text-xs" style="background: color-mix(in srgb, var(--color-warning, #f59e0b) 10%, transparent); color: var(--color-foreground)">
				{transitionTarget.dispute_reason}
			</p>
		{/if}
		{#if transitionError}
			<p
				class="mt-2 rounded-lg p-2 text-xs"
				style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
				{transitionError}
			</p>
		{/if}
		<div class="mt-3">
			<label for="bk-status" class="block text-xs font-medium" style="color: var(--color-muted)">
				{t('app.service_bookings.new_status', 'New status')}
			</label>
			<AutocompleteSelect
				id="bk-status"
				value={transitionStatus}
				onValueChange={(v) => (transitionStatus = v)}
				options={(transitionTarget ? (ADMIN_TARGETS[transitionTarget.status] ?? []) : []).map((s) => ({
					value: s,
					label: t(`app.service_bookings.status_${s}`, s)
				}))}
			/>
		</div>
		<div class="mt-3">
			<label for="bk-note" class="block text-xs font-medium" style="color: var(--color-muted)">
				{t('app.service_bookings.resolution_note', 'Resolution note (shown to the org)')}
			</label>
			<textarea
				id="bk-note"
				bind:value={resolutionNote}
				rows="3"
				class="mt-1 w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder={t('app.service_bookings.resolution_note_ph', 'Optional note…')}
			></textarea>
		</div>
		<div class="mt-4 flex justify-end gap-2">
			<button
				type="button"
				onclick={() => (transitionTarget = null)}
				class="rounded-lg border px-4 py-2 text-sm font-medium"
				style="border-color: var(--color-border); color: var(--color-foreground)"
			>{t('common.cancel', 'Cancel')}</button>
			<button
				type="button"
				onclick={submitTransition}
				disabled={acting || !transitionStatus}
				class="rounded-lg px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				style="background: var(--color-primary)"
			>{t('common.apply', 'Apply')}</button>
		</div>
	{/if}
</Modal>

<!-- ── View modal body ──────────────────────────────────────────────── -->
{#snippet BookingView({ row }: { row: BookingItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_listing', 'Listing')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.listing_title || '—'}</p>
		</div>
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_booker', 'Booker')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">
					{row.booker_username
						? `@${row.booker_username}${row.booker_display_name ? ` · ${row.booker_display_name}` : ''}`
						: '—'}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_org', 'Organization')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.org_name || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_provider', 'Provider')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">
					{row.provider_username
						? `@${row.provider_username}${row.provider_display_name ? ` · ${row.provider_display_name}` : ''}`
						: '—'}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_intent', 'Type')}</p>
				<p class="mt-1 text-sm capitalize" style="color: var(--color-foreground)">{t(`app.service_bookings.intent_${row.intent}`, row.intent)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_status', 'Status')}</p>
				<p class="mt-1 text-sm capitalize" style="color: var(--color-foreground)">{t(`app.service_bookings.status_${row.status}`, row.status)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_party_size', 'Party size')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.party_size ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_purchase_mode', 'Purchase mode')}</p>
				<p class="mt-1 text-sm capitalize" style="color: var(--color-foreground)">{row.purchase_mode ? t(`app.service_bookings.pmode_${row.purchase_mode}`, row.purchase_mode) : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_kind', 'Kind')}</p>
				<p class="mt-1 text-sm capitalize" style="color: var(--color-foreground)">
					{row.listing_kind ? t(`app.service_bookings.kind_${row.listing_kind}`, row.listing_kind) : '—'}
				</p>
			</div>
			{#if row.destination_space_name}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_destination', 'Destination')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.destination_space_name}</p>
				</div>
			{/if}
			{#if row.tier_name}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_tier', 'Tier')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.tier_name}</p>
				</div>
			{/if}
		</div>
		{#if row.departure_starts_on}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_departure', 'Departure')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">
					{new Date(row.departure_starts_on).toLocaleDateString()}{row.departure_ends_on ? ` → ${new Date(row.departure_ends_on).toLocaleDateString()}` : ''}
				</p>
			</div>
		{/if}
		{#if row.preferred_dates && (row.preferred_dates.start_on || row.preferred_dates.end_on)}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_pref_dates', 'Preferred dates')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">
					{row.preferred_dates.start_on ? new Date(row.preferred_dates.start_on).toLocaleDateString() : '—'} → {row.preferred_dates.end_on ? new Date(row.preferred_dates.end_on).toLocaleDateString() : '—'}{row.preferred_dates.flexible ? ` (${t('app.service_bookings.flexible', 'flexible')})` : ''}
				</p>
			</div>
		{/if}
		{#if row.total_price != null || row.fee_amount != null}
			<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_total', 'Total')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{formatMoney(row.total_price, row.fee_currency, null, { minor: true }) || '—'}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_fee', 'Platform fee')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{formatMoney(row.fee_amount, row.fee_currency, null, { minor: true }) || '—'}</p>
				</div>
				{#if row.payment_status}
					<div>
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_payment', 'Payment')}</p>
						<p class="mt-1 text-sm capitalize" style="color: var(--color-foreground)">{t(`app.service_bookings.pstatus_${row.payment_status}`, row.payment_status)}{#if row.payment_provider} · {row.payment_provider}{/if}</p>
					</div>
				{/if}
				{#if row.coupon_code}
					<div>
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_coupon', 'Coupon')}</p>
						<p class="mt-1 text-sm" style="color: var(--color-foreground)">
							{row.coupon_code}{#if row.discount_amount != null} · −{formatMoney(row.discount_amount, row.fee_currency, null, { minor: true })}{/if}
						</p>
					</div>
				{/if}
				{#if row.agent_balance_due != null}
					<div>
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_balance_due', 'Org balance due')}</p>
						<p class="mt-1 text-sm" style="color: var(--color-foreground)">{formatMoney(row.agent_balance_due, row.fee_currency, null, { minor: true })}</p>
					</div>
				{/if}
			</div>
		{/if}
		{#if row.note}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_note', 'Booker note')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.note}</p>
			</div>
		{/if}
		{#if row.org_response_note}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_org_note', 'Org response')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.org_response_note}</p>
			</div>
		{/if}
		{#if row.dispute_reason}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_dispute', 'Dispute reason')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-danger)">{row.dispute_reason}</p>
			</div>
		{/if}
		{#if row.cancel_reason}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_cancel_reason', 'Cancel reason')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.cancel_reason}</p>
			</div>
		{/if}
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_created', 'Created')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{fmtDate(row.created_at)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_expires', 'Expires')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{fmtDate(row.expires_at)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_confirmed', 'Confirmed')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{fmtDate(row.confirmed_at)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.service_bookings.col_fulfilled', 'Fulfilled')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{fmtDate(row.fulfilled_at)}</p>
			</div>
		</div>
	</div>
{/snippet}
