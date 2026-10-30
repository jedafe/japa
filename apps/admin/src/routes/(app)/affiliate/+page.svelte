<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /affiliate — migrated to shared components (ActionMenu, BulkBar, Modal, FilterPanel).
	// Tabbed page (6 tabs) — not an EntityPage candidate; keeps the tab structure but
	// uses DataTable `actions`/`bulkActionsList`/`filters` props and Modal for create.
	import { api, affiliateProvidersAdminApi, affiliateProgramAdminApi, affiliateLinksAdminApi, affiliateEnhancementsAdminApi, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import type { ActionItem, BulkAction, FilterDef } from '$lib/components/ui/data-table';
	import { Plus, Trash2, Link2, Users, Percent, DollarSign, BarChart3, Pencil } from 'lucide-svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	type AffiliateProvider = {
		id: string;
		name: string;
		slug: string;
		description: string | null;
		provider_type: string;
		base_url: string | null;
		category: string | null;
		logo_media_id: string | null;
		tracking_param: string | null;
		tracking_value: string | null;
		commission_rate: number | string | null;
		is_active: boolean;
		is_featured: boolean;
		created_at: string;
		updated_at: string;
	};

	type CommissionRates = { pro_monthly: number; pro_annual: number; course: number };

	type AffiliateMember = {
		id: string;
		user_id: string;
		username: string | null;
		enrolled_at: string;
		status: string;
		total_clicks: number;
		total_conversions: number;
		total_earned_credits: number;
		payout_method: string | null;
		notes: string | null;
	};

	type AffiliateLink = {
		id: string;
		user_id: string;
		username: string | null;
		product_type: string;
		product_id: string | null;
		code: string;
		description: string | null;
		destination_url: string;
		click_count: number;
		conversion_count: number;
		total_earned: number;
		is_active: boolean;
		created_at: string;
	};

	type ScanResult = {
		id: string;
		content_id: string;
		url: string;
		domain: string;
		detection_method: string;
		action_taken: string;
		created_at: string;
	};

	type AuthorCommission = {
		id: string;
		content_id: string;
		author_user_id: string;
		author_username: string | null;
		provider_name: string | null;
		external_commission: number;
		author_share: number;
		author_commission_pct: number;
		credited: boolean;
		created_at: string;
	};

	type ProgramStats = {
		total_members: number;
		active_members: number;
		total_links: number;
		total_clicks: number;
		total_conversions: number;
		total_commissions: string;
	};

	let activeTab = $state('overview');
	let providers = $state<AffiliateProvider[]>([]);
	let members = $state<AffiliateMember[]>([]);
	let links = $state<AffiliateLink[]>([]);
	let rates = $state<CommissionRates>({ pro_monthly: 0, pro_annual: 0, course: 0 });
	let scanResults = $state<ScanResult[]>([]);
	let commissions = $state<AuthorCommission[]>([]);
	let stats = $state<ProgramStats | null>(null);
	let loading = $state(true);
	let error = $state('');
	let showCreateModal = $state(false);
	let newProvider = $state({ name: '', base_url: '', category: '', commission_rate: 0 });
	// Provider edit — PATCH UpdateProvider rewrites every column, so the form
	// seeds from the full row (GET /admin/affiliate-providers/{id}) and the
	// payload carries the complete field set except slug (not in the
	// update SQL). A partial PATCH would NULL tracking/commission fields.
	let showEditProviderModal = $state(false);
	let editProviderTarget = $state<AffiliateProvider | null>(null);
	let editProviderLoading = $state(false);
	let editProvider = $state({
		name: '',
		description: '',
		provider_type: 'affiliate',
		base_url: '',
		category: '',
		logo_media_id: '',
		tracking_param: '',
		tracking_value: '',
		commission_rate: '',
		is_active: true,
		is_featured: false
	});
	let saving = $state(false);
	// The admin affiliate backends expose no ?q= search — the DataTable
	// search boxes filter the loaded rows client-side per tab instead of
	// re-fetching (a re-fetch would silently ignore the query).
	let searchQuery = $state('');

	function matchesQuery(row: Record<string, unknown>, q: string): boolean {
		const needle = q.trim().toLowerCase();
		if (!needle) return true;
		return Object.values(row).some((v) => v != null && String(v).toLowerCase().includes(needle));
	}

	const filteredProviders = $derived(providers.filter((r) => matchesQuery(r as unknown as Record<string, unknown>, searchQuery)));
	const filteredMembers = $derived(members.filter((r) => matchesQuery(r as unknown as Record<string, unknown>, searchQuery)));
	const filteredLinks = $derived(links.filter((r) => matchesQuery(r as unknown as Record<string, unknown>, searchQuery)));
	const filteredScanResults = $derived(scanResults.filter((r) => matchesQuery(r as unknown as Record<string, unknown>, searchQuery)));
	const filteredCommissions = $derived(commissions.filter((r) => matchesQuery(r as unknown as Record<string, unknown>, searchQuery)));

	let selectedProviderIds = $state<Set<string>>(new Set());
	let selectedMemberIds = $state<Set<string>>(new Set());
	let selectedLinkIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	// Filters (FilterPanel) — values are passed as query params to the API
	// for server-side filtering (BUG-006).
	let memberFilterValue = $state<Record<string, string>>({});
	let scanFilterValue = $state<Record<string, string>>({});
	let commissionFilterValue = $state<Record<string, string>>({});

	const canManageProviders = $derived(hasPermission('affiliate.providers.manage'));
	const canManageProgram = $derived(hasPermission('affiliate.admin.manage'));

	// Confirm modals (short confirmations — kept as Modal per recipe)
	let showDeleteProviderConfirm = $state(false);
	let deleteProviderTarget = $state<AffiliateProvider | null>(null);
	let showMemberStatusConfirm = $state(false);
	let memberStatusTarget = $state<{ member: AffiliateMember; status: string } | null>(null);

	// Record Commission Modal (POST /admin/affiliate/commissions — BUG-014)
	let showRecordCommissionModal = $state(false);
	let newCommission = $state({ provider_id: '', content_id: '', external_commission: 0, event_description: '' });

	const tabs = $derived([
		{ value: 'overview', label: t('affiliate.overview', 'Overview'), icon: BarChart3 },
		{ value: 'providers', label: t('affiliate.providers'), icon: Link2 },
		{ value: 'members', label: t('affiliate.members'), icon: Users },
		{ value: 'links', label: t('affiliate.links'), icon: Link2 },
		{ value: 'rates', label: t('affiliate.commission_rates'), icon: Percent },
		{ value: 'scanner', label: t('affiliate.link_scanner', 'Link Scanner'), icon: Link2 },
		{ value: 'author_commissions', label: t('affiliate.author_commissions', 'Author Commissions'), icon: DollarSign }
	]);

	let affiliateStatusOptions = $state<CatalogOption[]>([
		{ value: 'active', label: t('affiliate.active') },
		{ value: 'suspended', label: t('affiliate.suspended', 'Suspended') },
		{ value: 'banned', label: t('affiliate.banned', 'Banned') }
	]);

	let affiliateActionTakenOptions = $state<CatalogOption[]>([
		{ value: 'flagged', label: t('affiliate.flagged', 'Flagged') },
		{ value: 'blocked', label: t('affiliate.blocked', 'Blocked') },
		{ value: 'approved', label: t('affiliate.approved', 'Approved') }
	]);

	let affiliateCreditedOptions = $state<CatalogOption[]>([
		{ value: 'true', label: t('affiliate.credited', 'Credited') },
		{ value: 'false', label: t('affiliate.pending', 'Pending') }
	]);

	$effect(() => {
		void (async () => {
			const [statusRes, actionRes, creditedRes] = await Promise.all([
				adminCatalogApi.get('affiliate.status'),
				adminCatalogApi.get('affiliate.action_taken'),
				adminCatalogApi.get('affiliate.credited')
			]);
			if (statusRes.ok && statusRes.data?.length) affiliateStatusOptions = statusRes.data;
			if (actionRes.ok && actionRes.data?.length) affiliateActionTakenOptions = actionRes.data;
			if (creditedRes.ok && creditedRes.data?.length) affiliateCreditedOptions = creditedRes.data;
		})();
	});

	const memberFilters = $derived<FilterDef[]>([
		{
			key: 'status',
			label: t('affiliate.status'),
			options: affiliateStatusOptions
		}
	]);

	const scanFilters = $derived<FilterDef[]>([
		{
			key: 'action_taken',
			label: t('affiliate.action', 'Action'),
			options: affiliateActionTakenOptions
		}
	]);

	const commissionFilters = $derived<FilterDef[]>([
		{
			key: 'credited',
			label: t('affiliate.credited', 'Credited'),
			options: affiliateCreditedOptions
		}
	]);

	const providerColumns = $derived([
		{ key: 'name', label: t('affiliate.name'), sortable: true, render: (value: string) => `<span class="font-medium text-foreground">${value ?? '—'}</span>` },
		{ key: 'base_url', label: t('affiliate.base_url'), render: (value: string | null) => value ? `<span class="font-mono text-xs text-muted">${value}</span>` : '<span class="text-xs text-muted">—</span>' },
		{ key: 'category', label: t('affiliate.category', 'Category'), render: (value: string | null) => value ? `<span class="text-sm">${value}</span>` : '<span class="text-xs text-muted">—</span>' },
		{ key: 'commission_rate', label: t('affiliate.commission_rate'), sortable: true, render: (value: number | null) => value != null ? `<span class="font-medium">${value}%</span>` : '<span class="text-xs text-muted">—</span>' },
		{
			key: 'is_active',
			label: t('affiliate.status'),
			render: (_: any, row: any) => {
				const activeLabel = t('affiliate.active');
				const inactiveLabel = t('affiliate.inactive');
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${row.is_active ? 'bg-success/15 text-success border-success' : 'bg-surface text-foreground border'}">${row.is_active ? activeLabel : inactiveLabel}</span>`;
			}
		}
	]);

	const memberColumns = $derived([
		{ key: 'username', label: t('affiliate.username'), sortable: true, render: (value: string | null, row: any) => `<span class="font-medium">${value ?? row.user_id ?? '—'}</span>` },
		{
			key: 'status',
			label: t('affiliate.status'),
			render: (value: string) => {
				const classes: Record<string, string> = { active: 'bg-success/15 text-success border-success', suspended: 'bg-danger/15 text-danger border-danger', banned: 'bg-danger/15 text-danger border-danger' };
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[value] || 'bg-surface text-foreground border'}">${value}</span>`;
			}
		},
		{ key: 'total_earned_credits', label: t('affiliate.earnings'), sortable: true, render: (value: number) => `<span class="font-medium">${(value ?? 0).toFixed(2)}</span>` },
		{ key: 'total_conversions', label: t('affiliate.conversions'), sortable: true, render: (value: number) => `<span>${value ?? 0}</span>` }
	]);

	const linkColumns = $derived([
		{ key: 'username', label: t('affiliate.user'), sortable: true, render: (value: string | null, row: any) => `<span class="font-medium">${value ?? row.user_id ?? '—'}</span>` },
		{ key: 'code', label: t('affiliate.code'), sortable: true, render: (value: string) => `<span class="font-mono text-xs font-medium text-foreground">${value}</span>` },
		{ key: 'destination_url', label: t('affiliate.url'), render: (value: string) => `<span class="text-xs text-muted truncate max-w-[200px] block">${value}</span>` },
		{ key: 'click_count', label: t('affiliate.clicks'), sortable: true, render: (value: number) => `<span class="font-medium">${(value ?? 0).toLocaleString()}</span>` },
		{ key: 'conversion_count', label: t('affiliate.conversions'), sortable: true, render: (value: number) => `<span class="font-medium">${value ?? 0}</span>` },
		{ key: 'created_at', label: t('affiliate.created'), sortable: true, render: (value: string) => `<span class="text-sm">${new Date(value).toLocaleDateString()}</span>` }
	]);

	const scanColumns = $derived([
		{ key: 'url', label: t('affiliate.url', 'URL'), render: (value: string) => `<span class="font-mono text-xs truncate max-w-[300px] block">${value}</span>` },
		{ key: 'domain', label: t('affiliate.domain', 'Domain'), sortable: true, render: (value: string) => `<span class="text-sm font-medium">${value}</span>` },
		{ key: 'detection_method', label: t('affiliate.detection', 'Detection'), render: (value: string) => `<span class="inline-flex items-center rounded-full border px-2 py-0.5 text-xs" style="border-color: var(--color-border); color: var(--color-muted)">${value}</span>` },
		{
			key: 'action_taken',
			label: t('affiliate.action', 'Action'),
			render: (value: string) => {
				const cls = value === 'approved' ? 'bg-success/15 text-success border-success' : value === 'blocked' ? 'bg-danger/15 text-danger border-danger' : 'bg-warning/15 text-warning border-warning';
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${value}</span>`;
			}
		},
		{ key: 'created_at', label: t('affiliate.created'), sortable: true, render: (value: string) => `<span class="text-xs">${new Date(value).toLocaleDateString()}</span>` }
	]);

	const commissionColumns = $derived([
		{ key: 'provider_name', label: t('affiliate.provider', 'Provider'), sortable: true, render: (value: string | null) => `<span class="font-medium">${value ?? '—'}</span>` },
		{ key: 'author_username', label: t('affiliate.author', 'Author'), sortable: true, render: (value: string | null, row: any) => `<span class="font-medium">${value ?? row.author_user_id ?? '—'}</span>` },
		{ key: 'external_commission', label: t('affiliate.external_commission', 'External Comm.'), sortable: true, render: (value: number) => `<span class="font-medium">${(value ?? 0).toFixed(2)}</span>` },
		{ key: 'author_share', label: t('affiliate.author_share', 'Author Share'), sortable: true, render: (value: number) => `<span class="font-medium" style="color: var(--color-success)">${(value ?? 0).toFixed(2)}</span>` },
		{ key: 'author_commission_pct', label: t('affiliate.rate', 'Rate'), render: (value: number) => `<span>${value ?? 0}%</span>` },
		{
			key: 'credited',
			label: t('affiliate.credited', 'Credited'),
			render: (value: boolean) => {
				const cls = value ? 'bg-success/15 text-success border-success' : 'bg-warning/15 text-warning border-warning';
				const label = value ? t('common.yes', 'Yes') : t('affiliate.pending', 'Pending');
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${label}</span>`;
			}
		},
		{ key: 'created_at', label: t('affiliate.created'), sortable: true, render: (value: string) => `<span class="text-xs">${new Date(value).toLocaleDateString()}</span>` }
	]);

	// ── Row actions via ActionMenu ──────────────────────────────────────────
	const providerTypeOptions = [
		{ value: 'affiliate', label: t('affiliate.type_affiliate', 'Affiliate') },
		{ value: 'coupon', label: t('affiliate.type_coupon', 'Coupon') },
		{ value: 'both', label: t('affiliate.type_both', 'Both') }
	];

	const providerActions: ActionItem[] = [
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			show: () => canManageProviders,
			onclick: (row) => openEditProvider(row)
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			show: () => canManageProviders,
			onclick: (row) => confirmDeleteProvider(row)
		}
	];

	const memberActions: ActionItem[] = [
		{
			label: t('common.suspend', 'Suspend'),
			show: (row) => canManageProgram && row.status === 'active',
			onclick: (row) => confirmMemberStatus(row, 'suspended')
		},
		{
			label: t('common.activate', 'Activate'),
			show: (row) => canManageProgram && row.status !== 'active',
			onclick: (row) => confirmMemberStatus(row, 'active')
		}
	];

	// ── Bulk actions via BulkBar ────────────────────────────────────────────
	const providerBulkActions: BulkAction[] = [
		{ label: t('common.delete_selected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDeleteProviders }
	];
	const memberBulkActions: BulkAction[] = [
		{ label: t('affiliate.suspend', 'Suspend Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDeleteMembers }
	];
	const linkBulkActions: BulkAction[] = [
		{ label: t('common.delete_selected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDeleteLinks }
	];

	// Build query string from filter values for server-side filtering (BUG-006).
	function buildParams(filters: Record<string, string>): string {
		const params = new URLSearchParams();
		for (const [k, v] of Object.entries(filters)) {
			if (v) params.set(k, v);
		}
		return params.toString();
	}

	$effect(() => {
		load();
	});

	async function load() {
		loading = true;
		error = '';
		try {
			if (activeTab === 'overview') {
				const result = await affiliateProgramAdminApi.stats();
				if (result.ok) {
					stats = result.data as ProgramStats;
				} else {
					error = result.error;
				}
			} else if (activeTab === 'providers') {
				// `is_active` is intentionally absent — the admin list shows ALL
				// providers including inactive/pending-approval ones (§55).
				const result = await affiliateProvidersAdminApi.list('page_size=100');
				if (result.ok) {
					providers = (Array.isArray(result.data) ? result.data : []) as AffiliateProvider[];
				} else { error = result.error; }
			} else if (activeTab === 'members') {
				const params = buildParams(memberFilterValue);
				const result = await affiliateProgramAdminApi.members(params);
				if (result.ok) {
					members = (Array.isArray(result.data) ? result.data : []) as AffiliateMember[];
				} else { error = result.error; }
			} else if (activeTab === 'links') {
				const result = await affiliateLinksAdminApi.list();
				if (result.ok) {
					links = (Array.isArray(result.data) ? result.data : []) as AffiliateLink[];
				} else { error = result.error; }
			} else if (activeTab === 'rates') {
				const result = await affiliateProgramAdminApi.commissionRates();
				if (result.ok) {
					const data = result.data as any;
					rates = data?.rates ?? rates;
				} else { error = result.error; }
			} else if (activeTab === 'scanner') {
				const params = buildParams(scanFilterValue);
				const result = await affiliateEnhancementsAdminApi.scanResults(params);
				if (result.ok) {
					const data = result.data as any;
					scanResults = (Array.isArray(data) ? data : []) as ScanResult[];
				} else { error = result.error; }
			} else if (activeTab === 'author_commissions') {
				const params = buildParams(commissionFilterValue);
				const result = await affiliateEnhancementsAdminApi.commissions(params);
				if (result.ok) {
					const data = result.data as any;
					commissions = (Array.isArray(data) ? data : []) as AuthorCommission[];
				} else { error = result.error; }
			}
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function createProvider() {
		if (!newProvider.name || !newProvider.base_url) return;
		saving = true;
		try {
			const slug = newProvider.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
			const result = await affiliateProvidersAdminApi.create({ ...newProvider, category: newProvider.category || null, slug });
			if (result.ok) {
				showCreateModal = false;
				newProvider = { name: '', base_url: '', category: '', commission_rate: 0 };
				load();
			} else {
				error = result.error ?? t('common.error');
			}
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}

	// Fetch the full provider row before opening the edit form — PATCH is a
	// full-column overwrite, so a save seeded from a trimmed payload would
	// NULL the fields it doesn't carry (tracking_*, commission_rate…).
	async function openEditProvider(row: AffiliateProvider) {
		if (editProviderLoading) return;
		error = '';
		editProviderLoading = true;
		try {
			const result = await affiliateProvidersAdminApi.get(row.id);
			if (!result.ok) {
				error = result.error ?? t('common.error');
				return;
			}
			const p = result.data as AffiliateProvider | null;
			if (!p) {
				error = t('common.error');
				return;
			}
			editProviderTarget = p;
			editProvider = {
				name: p.name ?? '',
				description: p.description ?? '',
				provider_type: p.provider_type || 'affiliate',
				base_url: p.base_url ?? '',
				category: p.category ?? '',
				logo_media_id: p.logo_media_id ?? '',
				tracking_param: p.tracking_param ?? '',
				tracking_value: p.tracking_value ?? '',
				commission_rate: p.commission_rate != null ? String(p.commission_rate) : '',
				is_active: p.is_active,
				is_featured: p.is_featured
			};
			showEditProviderModal = true;
		} catch {
			error = t('common.error');
		} finally {
			editProviderLoading = false;
		}
	}

	async function saveProviderEdit() {
		if (!editProviderTarget || !editProvider.name.trim()) return;
		if (saving) return; // in-flight guard
		saving = true;
		try {
			const result = await affiliateProvidersAdminApi.update(editProviderTarget.id, {
				name: editProvider.name.trim(),
				description: editProvider.description.trim() || null,
				provider_type: editProvider.provider_type || editProviderTarget.provider_type,
				base_url: editProvider.base_url.trim() || null,
				category: editProvider.category.trim() || null,
				logo_media_id: editProvider.logo_media_id.trim() || null,
				tracking_param: editProvider.tracking_param.trim() || null,
				tracking_value: editProvider.tracking_value.trim() || null,
				commission_rate:
					editProvider.commission_rate !== '' ? Number(editProvider.commission_rate) : null,
				is_active: editProvider.is_active,
				is_featured: editProvider.is_featured
			});
			if (result.ok) {
				showEditProviderModal = false;
				editProviderTarget = null;
				load();
			} else {
				error = result.error ?? t('common.error');
			}
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function deleteProvider(id: string) {
		try {
			const result = await affiliateProvidersAdminApi.delete(id);
			if (result.ok) {
				providers = providers.filter((p) => p.id !== id);
			} else {
				error = result.error ?? t('common.error');
			}
		} catch {
			error = t('common.error');
		}
	}

	async function recordCommission() {
		if (!newCommission.provider_id || !newCommission.content_id || newCommission.external_commission <= 0) return;
		saving = true;
		try {
			const result = await affiliateEnhancementsAdminApi.recordCommission({
				provider_id: newCommission.provider_id,
				content_id: newCommission.content_id,
				external_commission: Number(newCommission.external_commission),
				event_description: newCommission.event_description || undefined
			});
			if (result.ok) {
				showRecordCommissionModal = false;
				newCommission = { provider_id: '', content_id: '', external_commission: 0, event_description: '' };
				load();
			} else {
				error = result.error ?? t('common.error');
			}
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function updateMemberStatus(userId: string, status: string) {
		try {
			const result = await affiliateProgramAdminApi.updateMemberStatus(userId, { status });
			if (result.ok) {
				load();
			} else {
				error = result.error ?? t('common.error');
			}
		} catch {
			error = t('common.error');
		}
	}

	async function saveRates() {
		saving = true;
		try {
			const result = await affiliateProgramAdminApi.updateCommissionRates(rates);
			if (!result.ok) {
				error = result.error ?? t('common.error');
			}
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}

	function confirmDeleteProvider(provider: AffiliateProvider) {
		deleteProviderTarget = provider;
		showDeleteProviderConfirm = true;
	}

	async function executeDeleteProvider() {
		if (!deleteProviderTarget) return;
		await deleteProvider(deleteProviderTarget.id);
		showDeleteProviderConfirm = false;
		deleteProviderTarget = null;
	}

	function confirmMemberStatus(member: AffiliateMember, status: string) {
		memberStatusTarget = { member, status };
		showMemberStatusConfirm = true;
	}

	async function executeMemberStatus() {
		if (!memberStatusTarget) return;
		await updateMemberStatus(memberStatusTarget.member.user_id, memberStatusTarget.status);
		showMemberStatusConfirm = false;
		memberStatusTarget = null;
	}

	async function bulkDeleteProviders(ids: Set<string>) {
		if (ids.size === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		try {
			await Promise.all(Array.from(ids).map((id) => affiliateProvidersAdminApi.delete(id)));
		} catch {
			error = t('common.error');
		} finally {
			bulkDeleting = false;
		}
	}

	async function bulkDeleteMembers(ids: Set<string>) {
		if (ids.size === 0) return;
		if (bulkDeleting) return;
		bulkDeleting = true;
		try {
			await Promise.all(Array.from(ids).map((userId) => updateMemberStatus(userId, 'suspended')));
		} catch {
			error = t('common.error');
		} finally {
			bulkDeleting = false;
		}
	}

	async function bulkDeleteLinks(ids: Set<string>) {
		if (ids.size === 0) return;
		if (bulkDeleting) return;
		bulkDeleting = true;
		try {
			await Promise.all(Array.from(ids).map((id) => api(`/admin/affiliate-links/${id}`, { method: 'DELETE' })));
		} catch {
			error = t('common.error');
		} finally {
			bulkDeleting = false;
		}
	}
</script>

<svelte:head>
	<title>{t('affiliate.title')} — {siteName()} Admin</title>
</svelte:head>

<PageHeader
	title={t('affiliate.title')}
	description={t('affiliate.description')}
	actions={activeTab === 'providers' && canManageProviders
		? [{ label: t('affiliate.add_provider'), icon: Plus, onclick: () => showCreateModal = true }]
		: []}
/>

<div class="space-y-6">
	{#if error}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{error}
		</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} tabParam="tab" tabPreserve={[]} />

	{#if loading}
		<p class="py-8 text-center text-sm text-muted">{t('common.loading')}</p>
	{:else if activeTab === 'overview'}
		{#if stats}
			<div class="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-6">
				<Card>
					<CardContent class="p-4">
						<p class="text-xs text-muted">{t('affiliate.total_members', 'Total Members')}</p>
						<p class="mt-1 text-2xl font-semibold text-foreground [font-variant-numeric:tabular-nums]">{stats.total_members}</p>
					</CardContent>
				</Card>
				<Card>
					<CardContent class="p-4">
						<p class="text-xs text-muted">{t('affiliate.active_members', 'Active Members')}</p>
						<p class="mt-1 text-2xl font-semibold text-foreground [font-variant-numeric:tabular-nums]">{stats.active_members}</p>
					</CardContent>
				</Card>
				<Card>
					<CardContent class="p-4">
						<p class="text-xs text-muted">{t('affiliate.total_links', 'Total Links')}</p>
						<p class="mt-1 text-2xl font-semibold text-foreground [font-variant-numeric:tabular-nums]">{stats.total_links}</p>
					</CardContent>
				</Card>
				<Card>
					<CardContent class="p-4">
						<p class="text-xs text-muted">{t('affiliate.total_clicks', 'Total Clicks')}</p>
						<p class="mt-1 text-2xl font-semibold text-foreground [font-variant-numeric:tabular-nums]">{stats.total_clicks}</p>
					</CardContent>
				</Card>
				<Card>
					<CardContent class="p-4">
						<p class="text-xs text-muted">{t('affiliate.total_conversions', 'Total Conversions')}</p>
						<p class="mt-1 text-2xl font-semibold text-foreground [font-variant-numeric:tabular-nums]">{stats.total_conversions}</p>
					</CardContent>
				</Card>
				<Card>
					<CardContent class="p-4">
						<p class="text-xs text-muted">{t('affiliate.total_commissions', 'Total Commissions')}</p>
						<p class="mt-1 text-2xl font-semibold text-foreground [font-variant-numeric:tabular-nums]">{stats.total_commissions}</p>
					</CardContent>
				</Card>
			</div>
		{:else}
			<p class="py-8 text-center text-sm text-muted">{t('affiliate.no_stats', 'No program stats available.')}</p>
		{/if}
	{:else if activeTab === 'providers'}
		<Card>
			<CardContent class="p-0">
				<DataTable
					columns={providerColumns}
					data={filteredProviders}
					pageSize={20}
					emptyMessage={t('affiliate.no_providers')}
					searchable
					onsearch={(q) => { searchQuery = q; }}
					selectable
					bind:selectedIds={selectedProviderIds}
					actions={providerActions}
					bulkActionsList={providerBulkActions}
				/>
			</CardContent>
		</Card>
	{:else if activeTab === 'members'}
		<Card>
			<CardContent class="p-0">
				<DataTable
					columns={memberColumns}
					data={filteredMembers}
					pageSize={20}
					emptyMessage={t('affiliate.no_members')}
					searchable
					onsearch={(q) => { searchQuery = q; }}
					selectable
					bind:selectedIds={selectedMemberIds}
					actions={memberActions}
					bulkActionsList={memberBulkActions}
					filters={memberFilters}
					bind:filterValue={memberFilterValue}
				/>
			</CardContent>
		</Card>
	{:else if activeTab === 'links'}
		<Card>
			<CardContent class="p-0">
				<DataTable
					columns={linkColumns}
					data={filteredLinks}
					pageSize={20}
					emptyMessage={t('affiliate.no_links')}
					searchable
					onsearch={(q) => { searchQuery = q; }}
					selectable
					bind:selectedIds={selectedLinkIds}
					bulkActionsList={linkBulkActions}
				/>
			</CardContent>
		</Card>
	{:else if activeTab === 'rates'}
		<Card>
			<CardContent class="p-6">
				<h3 class="mb-4 text-lg font-semibold text-foreground">{t('affiliate.commission_rates')}</h3>
				<div class="mx-auto max-w-lg space-y-4">
					<div>
						<label for="rate-monthly" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.pro_monthly')}</label>
						<div class="flex items-center gap-2">
							<input id="rate-monthly" type="number" min="0" max="100" step="0.1" class="w-24 rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={rates.pro_monthly} />
							<span class="text-sm text-muted">%</span>
						</div>
					</div>
					<div>
						<label for="rate-annual" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.pro_annual')}</label>
						<div class="flex items-center gap-2">
							<input id="rate-annual" type="number" min="0" max="100" step="0.1" class="w-24 rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={rates.pro_annual} />
							<span class="text-sm text-muted">%</span>
						</div>
					</div>
					<div>
						<label for="rate-course" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.course')}</label>
						<div class="flex items-center gap-2">
							<input id="rate-course" type="number" min="0" max="100" step="0.1" class="w-24 rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={rates.course} />
							<span class="text-sm text-muted">%</span>
						</div>
					</div>
					<Button onclick={saveRates} disabled={saving}>
						{saving ? t('common.saving') : t('common.save')}
					</Button>
				</div>
			</CardContent>
		</Card>
	{:else if activeTab === 'scanner'}
		<Card>
			<CardContent class="p-0">
				<DataTable
					columns={scanColumns}
					data={filteredScanResults}
					pageSize={20}
					emptyMessage={t('affiliate.no_scan_results', 'No unregistered affiliate links detected')}
					searchable
					onsearch={(q) => { searchQuery = q; }}
					filters={scanFilters}
					bind:filterValue={scanFilterValue}
				/>
			</CardContent>
		</Card>
	{:else if activeTab === 'author_commissions'}
		<Card>
			<CardContent class="p-0">
				{#if canManageProgram}
					<div class="flex justify-end p-3 border-b border-border">
						<Button onclick={() => (showRecordCommissionModal = true)}>
							<Plus size={16} />
							{t('affiliate.record_commission', 'Record Commission')}
						</Button>
					</div>
				{/if}
				<DataTable
					columns={commissionColumns}
					data={filteredCommissions}
					pageSize={20}
					emptyMessage={t('affiliate.no_commissions', 'No author commissions recorded yet')}
					searchable
					onsearch={(q) => { searchQuery = q; }}
					filters={commissionFilters}
					bind:filterValue={commissionFilterValue}
				/>
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Add Provider Modal -->
<Modal bind:open={showCreateModal} title={t('affiliate.add_provider')}>
	<div class="space-y-4">
		<div>
			<label for="provider-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.name')}</label>
			<input id="provider-name" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={newProvider.name} />
		</div>
		<div>
			<label for="provider-url" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.base_url')}</label>
			<input id="provider-url" type="url" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder="https://example.com" bind:value={newProvider.base_url} />
		</div>
		<div>
			<label for="provider-category" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.category', 'Category')}</label>
			<input id="provider-category" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder={t('affiliate.category_placeholder', 'Cloud, AI, SaaS, Dev…')} bind:value={newProvider.category} />
		</div>
		<div>
			<label for="provider-rate" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.commission_rate')}</label>
			<div class="flex items-center gap-2">
				<input id="provider-rate" type="number" min="0" max="100" step="0.1" class="w-24 rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={newProvider.commission_rate} />
				<span class="text-sm text-muted">%</span>
			</div>
		</div>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => showCreateModal = false}>{t('common.cancel')}</Button>
			<Button onclick={createProvider} disabled={saving || !newProvider.name || !newProvider.base_url}>
				{saving ? t('common.creating') : t('common.create')}
			</Button>
		</div>
	{/snippet}
</Modal>

<!-- Edit Provider Modal — full-column PATCH; every UpdateProviderInput
     field except slug is carried so the overwrite can't NULL hidden columns. -->
<Modal bind:open={showEditProviderModal} title={t('affiliate.edit_provider', 'Edit Provider')}>
	<div class="space-y-4">
		<div>
			<label for="edit-provider-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.name')}</label>
			<input id="edit-provider-name" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={editProvider.name} />
		</div>
		<div>
			<label for="edit-provider-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('common.description', 'Description')}</label>
			<textarea id="edit-provider-desc" rows="2" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={editProvider.description}></textarea>
		</div>
		<div>
			<label for="edit-provider-type" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.provider_type', 'Provider Type')}</label>
			<AutocompleteSelect
				id="edit-provider-type"
				bind:value={editProvider.provider_type}
				options={providerTypeOptions}
				class="w-full"
			/>
		</div>
		<div>
			<label for="edit-provider-url" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.base_url')}</label>
			<input id="edit-provider-url" type="url" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder="https://example.com" bind:value={editProvider.base_url} />
		</div>
		<div>
			<label for="edit-provider-category" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.category', 'Category')}</label>
			<input id="edit-provider-category" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder={t('affiliate.category_placeholder', 'Cloud, AI, SaaS, Dev…')} bind:value={editProvider.category} />
		</div>
		<div>
			<label for="edit-provider-logo" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.logo_media_id', 'Logo Media ID')}</label>
			<input id="edit-provider-logo" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 font-mono text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder="UUID" bind:value={editProvider.logo_media_id} />
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="edit-provider-tp" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.tracking_param', 'Tracking Param')}</label>
				<input id="edit-provider-tp" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={editProvider.tracking_param} />
			</div>
			<div>
				<label for="edit-provider-tv" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.tracking_value', 'Tracking Value')}</label>
				<input id="edit-provider-tv" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={editProvider.tracking_value} />
			</div>
		</div>
		<div>
			<label for="edit-provider-rate" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.commission_rate')}</label>
			<div class="flex items-center gap-2">
				<input id="edit-provider-rate" type="number" min="0" max="100" step="0.1" class="w-24 rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={editProvider.commission_rate} />
				<span class="text-sm text-muted">%</span>
			</div>
		</div>
		<div class="flex items-center gap-6">
			<label class="flex items-center gap-2 text-sm text-foreground">
				<input type="checkbox" bind:checked={editProvider.is_active} class="h-4 w-4 rounded" />
				{t('affiliate.is_active', 'Active')}
			</label>
			<label class="flex items-center gap-2 text-sm text-foreground">
				<input type="checkbox" bind:checked={editProvider.is_featured} class="h-4 w-4 rounded" />
				{t('affiliate.is_featured', 'Featured')}
			</label>
		</div>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => (showEditProviderModal = false)}>{t('common.cancel')}</Button>
			<Button onclick={saveProviderEdit} disabled={saving || !editProvider.name.trim()}>
				{saving ? t('common.saving') : t('common.save')}
			</Button>
		</div>
	{/snippet}
</Modal>

<!-- Delete Provider Confirm -->
<Modal bind:open={showDeleteProviderConfirm} title={t('common.confirmDelete', 'Confirm Delete')}>
	{#if deleteProviderTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('affiliate.delete_provider_confirm', 'Delete provider')} <strong>{deleteProviderTarget.name}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t('common.cannotUndo', 'This action cannot be undone.')}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showDeleteProviderConfirm = false}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={executeDeleteProvider}>{t('common.delete', 'Delete')}</Button>
	</div>
</Modal>

<!-- Member Status Confirm -->
<Modal bind:open={showMemberStatusConfirm} title={t('common.confirm', 'Confirm')}>
	{#if memberStatusTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('affiliate.change_member_status_confirm', 'Change status of')} <strong>{memberStatusTarget.member.username ?? memberStatusTarget.member.user_id}</strong> {t('affiliate.to', 'to')} <strong>{memberStatusTarget.status}</strong>?
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showMemberStatusConfirm = false}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={executeMemberStatus}>{t('common.confirm', 'Confirm')}</Button>
	</div>
</Modal>

<!-- Record Commission Modal (POST /admin/affiliate/commissions — BUG-014) -->
<Modal bind:open={showRecordCommissionModal} title={t('affiliate.record_commission', 'Record Commission')}>
	<div class="space-y-4">
		<div>
			<label for="comm-provider" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.provider', 'Provider')}</label>
			<AutocompleteSelect
				id="comm-provider"
				bind:value={newCommission.provider_id}
				options={[{ value: '', label: t('affiliate.select_provider', 'Select provider…') }, ...providers.map((p) => ({ value: p.id, label: p.name }))]}
				class="w-full"
			/>
		</div>
		<div>
			<label for="comm-content" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.content_id', 'Content ID')}</label>
			<input id="comm-content" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm font-mono placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder="UUID" bind:value={newCommission.content_id} />
		</div>
		<div>
			<label for="comm-amount" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.external_commission', 'External Commission (NGN)')}</label>
			<input id="comm-amount" type="number" min="0" step="0.01" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={newCommission.external_commission} />
		</div>
		<div>
			<label for="comm-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.event_description', 'Event Description')}</label>
			<textarea id="comm-desc" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" rows="3" bind:value={newCommission.event_description}></textarea>
		</div>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => (showRecordCommissionModal = false)}>{t('common.cancel')}</Button>
			<Button onclick={recordCommission} disabled={saving || !newCommission.provider_id || !newCommission.content_id || newCommission.external_commission <= 0}>
				{saving ? t('common.saving') : t('common.record', 'Record')}
			</Button>
		</div>
	{/snippet}
</Modal>
