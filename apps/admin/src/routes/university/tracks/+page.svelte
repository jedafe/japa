<script lang="ts">
	// /university/tracks — migrated to the EntityPage scaffold.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { universityAdminApi, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { loadCurrencies, formatMoney, type CurrencyInfo } from '$lib/utils/currency';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Check, Eye, X } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type Track = {
		id: string;
		title: string;
		slug: string;
		description?: string | null;
		difficulty: string;
		instructor_id?: string | null;
		instructor_name: string;
		price_kobo?: number | null;
		currency: string;
		category?: string | null;
		language: string;
		geo_space_id?: string | null;
		total_enrolled: number;
		is_published: boolean;
		is_published_to_university: boolean;
		university_submission_status: string;
		created_at: string;
		updated_at: string;
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let actionLoading = $state(false);

	let actionTarget = $state<Track | null>(null);
	let showApproveConfirm = $state(false);
	let showRejectModal = $state(false);
	let rejectReason = $state('');

	const canApprove = $derived(hasPermission('university.course.approve'));

	// §58 — managed currency symbols for price display.
	let currencies = $state<CurrencyInfo[]>([]);
	loadCurrencies().then((list) => (currencies = list));

	function formatPrice(kobo: number | null | undefined, currency: string): string {
		if (kobo == null) return t('university.free', 'Free');
		return formatMoney(kobo, currency, currencies, { minor: true, fractionDigits: 0, withCode: true });
	}

	function openApprove(row: Track) { actionTarget = row; showApproveConfirm = true; }
	function openReject(row: Track) { actionTarget = row; rejectReason = ''; showRejectModal = true; }

	async function confirmApprove() {
		if (!actionTarget) return;
		actionLoading = true;
		pageError = '';
		try {
			const result = await universityAdminApi.approveTrack(actionTarget.id);
			if (result.ok) {
				showApproveConfirm = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('university.approve_failed', 'Failed to approve track');
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('university.approve_failed', 'Failed to approve track');
		} finally {
			actionLoading = false;
		}
	}

	async function confirmReject() {
		if (!actionTarget) return;
		actionLoading = true;
		pageError = '';
		try {
			const result = await universityAdminApi.rejectTrack(actionTarget.id, { reason: rejectReason.trim() || undefined });
			if (result.ok) {
				showRejectModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('university.reject_failed', 'Failed to reject track');
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('university.reject_failed', 'Failed to reject track');
		} finally {
			actionLoading = false;
		}
	}

	// `fields` is required by EntityPageConfig; tracks are read-only in this view.
	const fields: FieldDef[] = [
		{ key: 'title', label: t('university.track_title', 'Track'), type: 'text' },
		{ key: 'slug', label: t('university.slug', 'Slug'), type: 'text' },
		{ key: 'difficulty', label: t('university.difficulty', 'Difficulty'), type: 'text' },
		{ key: 'price_kobo', label: t('university.price', 'Price'), type: 'number' }
	];

	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: Track, ctx?: any) => ctx?.openView(row) }
		];
		if (canApprove) {
			acts.push({
				label: t('common.approve', 'Approve'),
				icon: Check,
				onclick: openApprove,
				show: (row: Track) => row.university_submission_status === 'pending'
			});
			acts.push({
				label: t('common.reject', 'Reject'),
				icon: X,
				variant: 'destructive',
				onclick: openReject,
				show: (row: Track) => row.university_submission_status === 'pending'
			});
		}
		return acts;
	});

	let statusOptions = $state<CatalogOption[]>([]);
	let difficultyOptions = $state<CatalogOption[]>([]);

	async function loadFilterOptions() {
		const [statusRes, difficultyRes] = await Promise.all([
			adminCatalogApi.get('university.tracks.status'),
			adminCatalogApi.get('university.tracks.difficulty')
		]);
		if (statusRes.ok && statusRes.data) {
			statusOptions = statusRes.data;
		}
		if (difficultyRes.ok && difficultyRes.data) {
			difficultyOptions = difficultyRes.data;
		}
	}

	$effect(() => {
		loadFilterOptions();
	});

	const statusFilterOptions = $derived([
		{ value: '', label: t('university.all', 'All') },
		...statusOptions
	]);

	const config = $derived<EntityPageConfig>({
		title: t('university.tracks_title', 'University Track Approvals'),
		description: t('university.tracks_desc', 'Review and approve instructor-submitted tracks for the Japa University marketplace'),
		pageSize: 25,
		readonly: true,
		columns: [
			{ key: 'title', label: t('university.track_title', 'Track'), sortable: true, render: (v: string) => `<span class="font-medium">${v}</span>` },
			{ key: 'instructor_name', label: t('university.instructor', 'Instructor'), sortable: true },
			{
				key: 'difficulty',
				label: t('university.difficulty', 'Difficulty'),
				sortable: true,
				render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v === 'advanced' ? 'bg-danger/15 text-danger' : v === 'intermediate' ? 'bg-warning/15 text-warning' : 'bg-success/15 text-success'}">${v}</span>`
			},
			{ key: 'price_kobo', label: t('university.price', 'Price'), sortable: true, render: (_: any, row: Track) => `<span class="text-xs">${formatPrice(row.price_kobo, row.currency)}</span>` },
			{ key: 'category', label: t('university.category', 'Category'), sortable: true, render: (v: string | null) => `<span class="text-xs" style="color: var(--color-muted)">${v ?? '—'}</span>` },
			{ key: 'language', label: t('university.language', 'Language'), sortable: true, render: (v: string) => `<span class="text-xs">${v}</span>` },
			{ key: 'total_enrolled', label: t('university.enrolled', 'Enrolled'), sortable: true, render: (v: number) => `<span class="text-xs">${v}</span>` },
			{ key: 'created_at', label: t('common.submitted', 'Submitted'), sortable: true, render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>` }
		],
		filters: [
			{
				key: 'status',
				label: t('university.status', 'Status'),
				options: statusFilterOptions
			},
			{
				key: 'difficulty',
				label: t('university.difficulty', 'Difficulty'),
				options: difficultyOptions
			}
		],
		fields,
		actions: rowActions,
		load: async ({ page, pageSize, q, filter, sort }) => {
			const status = filter.status ?? 'pending';
			const params = new URLSearchParams();
			// Load a large window server-side, then apply client-side search/difficulty/pagination
			// because the backend does not yet support text search or difficulty filters on these endpoints.
			params.set('page', '1');
			params.set('limit', '1000');
			if (status && status !== 'all') {
				if (status === 'pending') {
					// listPendingTracks does not take a status param.
				} else {
					params.set('status', status);
				}
			}
			if (sort) {
				const [key, dir] = sort.split(':');
				if (key) {
					params.set('sort', key);
					params.set('dir', dir ?? 'asc');
				}
			}
			const result = status === 'pending'
				? await universityAdminApi.listPendingTracks(params.toString())
				: await universityAdminApi.listAllTracks(params.toString());
			if (result.ok) {
				let data = result.data.data ?? [];
				// Client-side search
				const query = (q ?? '').trim().toLowerCase();
				if (query) {
					data = data.filter((tr) =>
						tr.title.toLowerCase().includes(query) ||
						tr.instructor_name.toLowerCase().includes(query) ||
						(tr.category ?? '').toLowerCase().includes(query)
					);
				}
				// Client-side difficulty filter
				if (filter.difficulty) {
					data = data.filter((tr) => tr.difficulty === filter.difficulty);
				}
				// Client-side sort (the backend sort may not match all needs)
				if (sort) {
					const [key, dir] = sort.split(':');
					if (key) {
						data = [...data].sort((a: any, b: any) => {
							const aVal = a[key] ?? '';
							const bVal = b[key] ?? '';
							const cmp = typeof aVal === 'string' ? aVal.localeCompare(bVal) : (aVal - bVal);
							return dir === 'desc' ? -cmp : cmp;
						});
					}
				}
				const total = data.length;
				const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				const start = page * pageSize;
				return { data: data.slice(start, start + pageSize), total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load tracks'));
		},
		viewSnippet: TrackView
	});
</script>

<svelte:head>
	<title>{t('university.tracks_title', 'University Track Approvals')} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- Approve Modal -->
{#if actionTarget}
	<Modal bind:open={showApproveConfirm} title={t('university.approve_track', 'Approve Track')}>
		<p class="text-sm" style="color: var(--color-muted)">{t('university.approve_track_confirm', 'Publish')} <strong>{actionTarget.title}</strong> {t('university.to_marketplace', 'to the Japa University marketplace?')}</p>
		<div class="mt-6 flex justify-end gap-2">
			<Button variant="outline" onclick={() => { showApproveConfirm = false; }}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={confirmApprove} disabled={actionLoading}>{actionLoading ? t('common.processing', 'Processing...') : t('common.approve', 'Approve')}</Button>
		</div>
	</Modal>
{/if}

<!-- Reject Modal -->
{#if actionTarget}
	<Modal bind:open={showRejectModal} title={t('university.reject_track', 'Reject Track')}>
		<p class="mb-3 text-sm" style="color: var(--color-muted)">{t('university.reject_track_confirm', 'Reject')} <strong>{actionTarget.title}</strong>?</p>
		<div>
			<label for="reject-reason" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('university.rejectReason', 'Reason (optional)')}</label>
			<textarea id="reject-reason" bind:value={rejectReason} rows={3} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder={t('university.rejectReasonPlaceholder', 'Reason for rejection...')}></textarea>
		</div>
		<div class="mt-6 flex justify-end gap-2">
			<Button variant="outline" onclick={() => { showRejectModal = false; }}>{t('common.cancel', 'Cancel')}</Button>
			<Button variant="destructive" onclick={confirmReject} disabled={actionLoading}>{actionLoading ? t('common.processing', 'Processing...') : t('common.reject', 'Reject')}</Button>
		</div>
	</Modal>
{/if}

{#snippet TrackView({ row }: { row: Track })}
	<div class="space-y-3">
		<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('university.track_title', 'Track')}</p><p class="mt-1 text-sm font-medium">{row.title}</p></div>
		<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('university.instructor', 'Instructor')}</p><p class="mt-1 text-sm">{row.instructor_name}</p></div>
		<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('university.slug', 'Slug')}</p><p class="mt-1 text-sm font-mono">{row.slug}</p></div>
		{#if row.description}
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('university.description', 'Description')}</p><p class="mt-1 text-sm whitespace-pre-wrap">{row.description}</p></div>
		{/if}
		<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('university.difficulty', 'Difficulty')}</p><p class="mt-1 text-sm">{row.difficulty}</p></div>
		<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('university.price', 'Price')}</p><p class="mt-1 text-sm">{formatPrice(row.price_kobo, row.currency)}</p></div>
		{#if row.category}
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('university.category', 'Category')}</p><p class="mt-1 text-sm">{row.category}</p></div>
		{/if}
		<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('university.language', 'Language')}</p><p class="mt-1 text-sm">{row.language}</p></div>
		<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('university.enrolled', 'Enrolled')}</p><p class="mt-1 text-sm">{row.total_enrolled}</p></div>
	</div>
{/snippet}
