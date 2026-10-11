<script lang="ts">
	// /newsletter — migrated to the EntityPage scaffold.
	// Tabbed page: Subscribers (view/edit/delete + import) and Campaigns
	// (view/send + bulk delete + create-via-modal), plus an Analytics tab
	// with stat cards and a campaign-performance bar chart.
	// Create-campaign and import-subscribers keep page-local Modals because
	// their forms use rich inputs (HTML textarea / bulk paste) that EntityForm
	// does not support; they are triggered via EntityPage `headerActions`.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Tabs } from '$lib/components/ui/tabs';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { BarChart } from 'layerchart';
	import { api } from '$lib/api';
	import { sanitizeHtml } from '$lib/utils/html';
	import { Mail, Plus, Send, Users, BarChart3, Trash2, Eye, Pencil, Upload } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	type Subscriber = {
		id: string;
		email: string;
		name?: string;
		status: string;
		subscribedAt: string;
		created_at: string;
	};

	type Campaign = {
		id: string;
		name: string;
		subject: string;
		preview?: string;
		status: string;
		sent_at?: string;
		sentAt?: string;
		recipient_count?: number;
		open_count?: number;
		click_count?: number;
		// Derived display fields (computed client-side from open_count/recipient_count).
		openRate?: number;
		clickRate?: number;
		created_at: string;
	};

	type NewsletterStats = {
		subscribers: number;
		campaigns: number;
		avgOpenRate: number;
		avgClickRate: number;
	};

	const canManageNewsletter = $derived(hasPermission('newsletter.manage'));

	let activeTab = $state('subscribers');

	// Stats for tab badges + analytics cards. Loaded once on mount and refreshed
	// when the analytics tab is opened. The EntityPage instances own their own
	// list data; this state is only for the cross-tab summary.
	let stats = $state<NewsletterStats>({ subscribers: 0, campaigns: 0, avgOpenRate: 0, avgClickRate: 0 });
	// Campaigns fetched for the analytics chart (independent of the campaigns
	// EntityPage, which owns its own paginated list).
	let analyticsCampaigns = $state<Campaign[]>([]);
	let analyticsLoading = $state(false);
	let pageError = $state('');

	// LayerChart axis format for the campaign-performance bar chart.
	const newsletterChartAxis: any = { x: { ticks: { rotate: -30 } } };

	const tabs = [
		{ value: 'subscribers', label: t('app.newsletter.tab_subscribers', 'Subscribers'), icon: Users, count: stats.subscribers },
		{ value: 'campaigns', label: t('app.newsletter.tab_campaigns', 'Campaigns'), icon: Mail, count: stats.campaigns },
		{ value: 'analytics', label: t('app.newsletter.tab_analytics', 'Analytics'), icon: BarChart3 }
	];

	const subscriberStatusOptions = [
		{ value: 'subscribed', label: t('app.newsletter.status_subscribed', 'Subscribed') },
		{ value: 'unsubscribed', label: t('app.newsletter.status_unsubscribed', 'Unsubscribed') },
		{ value: 'bounced', label: t('app.newsletter.status_bounced', 'Bounced') },
		{ value: 'complained', label: t('app.newsletter.status_complained', 'Complained') }
	];

	const campaignStatusOptions = [
		{ value: 'draft', label: t('app.newsletter.status_draft', 'Draft') },
		{ value: 'scheduled', label: t('app.newsletter.status_scheduled', 'Scheduled') },
		{ value: 'sending', label: t('app.newsletter.status_sending', 'Sending') },
		{ value: 'sent', label: t('app.newsletter.status_sent', 'Sent') },
		{ value: 'cancelled', label: t('app.newsletter.status_cancelled', 'Cancelled') }
	];

	// ── Controllers ─────────────────────────────────────────────────────────
	let subscriberController = $state<EntityPageController | undefined>(undefined);
	let campaignController = $state<EntityPageController | undefined>(undefined);

	// ── Campaign creation Modal (page-local — form with HTML textarea) ──────
	let showCreateModal = $state(false);
	let newCampaign = $state({ name: '', subject: '', html: '' });
	let creating = $state(false);

	// ── Import subscribers Modal (page-local — form with textarea) ──────────
	let showImportModal = $state(false);
	let importText = $state('');
	let importing = $state(false);
	let importResult = $state<{ success: number; failed: number; errors: string[] } | null>(null);

	// ── Bulk delete (campaigns) in-flight guard ─────────────────────────────
	let bulkDeleting = $state(false);

	// ── Filters ─────────────────────────────────────────────────────────────
	const subscriberFilters = $derived<FilterDef[]>([
		{ key: 'status', label: t('app.newsletter.filter_status', 'Status'), options: subscriberStatusOptions }
	]);
	const campaignFilters = $derived<FilterDef[]>([
		{ key: 'campaign_status', label: t('app.newsletter.filter_status', 'Status'), options: campaignStatusOptions }
	]);

	// ── Subscriber edit fields (status only — email is read-only) ───────────
	const subscriberFields = $derived<FieldDef[]>([
		{ key: 'status', label: t('common.status', 'Status'), type: 'select', options: subscriberStatusOptions }
	]);

	// ── Row actions ─────────────────────────────────────────────────────────
	const subscriberActions = $derived<ActionItem[]>([
		{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) },
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row, ctx) => ctx?.openEdit(row),
			show: () => canManageNewsletter
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row, ctx) => ctx?.openDelete(row),
			show: () => canManageNewsletter
		}
	]);

	const campaignActions = $derived<ActionItem[]>([
		{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) },
		{
			label: t('common.send', 'Send'),
			icon: Send,
			onclick: (row) => sendCampaign(row.id),
			show: (row: Campaign) => (row.status === 'draft' || row.status === 'scheduled') && canManageNewsletter
		}
	]);

	// ── Subscribers EntityPage config ───────────────────────────────────────
	const subscriberConfig = $derived<EntityPageConfig>({
		title: t('app.newsletter.tab_subscribers', 'Subscribers'),
		description: t('app.newsletter.empty_subscribers', 'Manage email subscribers'),
		// readonly hides the Create button (subscribers are added via import);
		// edit/delete are exposed through custom `actions` with show() predicates.
		readonly: true,
		pageSize: 20,
		columns: [
			{
				key: 'email',
				label: t('app.newsletter.col_email', 'Email'),
				render: (_: any, row: any) => `
					<div class="flex items-center gap-3">
						<div class="h-8 w-8 rounded-full bg-primary-100 flex items-center justify-center">
							<span class="text-sm font-medium text-primary-700">${(row.email || '?')[0].toUpperCase()}</span>
						</div>
						<div>
							<p class="font-medium text-foreground">${row.email}</p>
							${row.name ? `<p class="text-sm text-muted">${row.name}</p>` : ''}
						</div>
					</div>
				`
			},
			{
				key: 'status',
				label: t('app.newsletter.col_status', 'Status'),
				render: (v: string) => {
					const cls = v === 'subscribed' ? 'bg-success/15 text-success border-success' : v === 'bounced' ? 'bg-danger/15 text-danger border-danger' : v === 'complained' ? 'bg-warning/15 text-warning border-warning' : 'bg-surface text-foreground border';
					return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${v || 'subscribed'}</span>`;
				}
			},
			{
				key: 'subscribedAt',
				label: t('app.newsletter.col_subscribed', 'Subscribed'),
				render: (v: string, row: any) => new Date(v || row.created_at).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' })
			}
		],
		filters: subscriberFilters,
		actions: subscriberActions,
		fields: subscriberFields,
		headerActions: canManageNewsletter
			? [{ label: t('app.newsletter.btn_import_subscribers', 'Import Subscribers'), icon: Upload, variant: 'outline', onclick: openImportModal }]
			: [],
		load: async ({ page, pageSize, q, filter }) => {
			const result = await api<Subscriber[]>('/admin/newsletter/subscribers?page_size=1000');
			if (result.ok) {
				let data = result.data ?? [];
				const query = (q ?? '').toLowerCase();
				if (query) {
					data = data.filter((row) =>
						[row.email, row.name].some((v) =>
							String(v ?? '').toLowerCase().includes(query)
						)
					);
				}
				if (filter?.status) {
					data = data.filter((row) => row.status === filter.status);
				}
				const size = pageSize || 25;
				const offset = (page || 0) * size;
				const total = data.length;
				const totalPages = Math.max(1, Math.ceil(total / size));
				return {
					data: data.slice(offset, offset + size),
					total,
					totalPages
				};
			}
			throw new Error(result.error ?? t('app.newsletter.error_load_subscribers', 'Failed to load subscribers'));
		},
		update: async (id, body) => {
			const res = await api(`/admin/newsletter/subscribers/${id}`, { method: 'PATCH', body: { status: body.status } });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/newsletter/subscribers/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: SubscriberView
	});

	// ── Campaigns EntityPage config ─────────────────────────────────────────
	const campaignConfig = $derived<EntityPageConfig>({
		title: t('app.newsletter.tab_campaigns', 'Campaigns'),
		description: t('app.newsletter.empty_campaigns', 'Manage newsletter campaigns'),
		// readonly hides the Create button; campaigns are created via the
		// page-local Modal (rich HTML form) triggered by `headerActions`.
		readonly: true,
		pageSize: 20,
		columns: [
			{
				key: 'subject',
				label: t('app.newsletter.col_subject', 'Subject'),
				render: (_: any, row: any) => `
					<div>
						<p class="font-medium text-foreground">${row.subject || row.name || 'No subject'}</p>
						${row.preview ? `<p class="text-sm text-muted">${row.preview}</p>` : ''}
					</div>
				`
			},
			{
				key: 'status',
				label: t('app.newsletter.col_status', 'Status'),
				render: (v: string) => {
					const cls = v === 'sent' ? 'bg-success/15 text-success border-success' : v === 'scheduled' ? 'bg-info-100 text-info-800 border-info-200' : v === 'sending' ? 'bg-warning/15 text-warning border-warning' : v === 'cancelled' ? 'bg-danger/15 text-danger border-danger' : 'bg-surface text-foreground border';
					return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${v || 'draft'}</span>`;
				}
			},
			{
				key: 'sentAt',
				label: t('app.newsletter.col_sent', 'Sent'),
				render: (v: string, row: any) => {
					const date = v || row.sent_at;
					return date ? new Date(date).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' }) : '<span class="text-muted">—</span>';
				}
			},
			{
				key: 'openRate',
				label: t('app.newsletter.col_open_rate', 'Open Rate'),
				render: (v: number, row: any) => {
					const rate = v ?? row.openRate ?? 0;
					return `<span class="font-medium">${rate.toFixed(1)}%</span>`;
				}
			}
		],
		filters: campaignFilters,
		actions: campaignActions,
		fields: [],
		bulkActions: canManageNewsletter
			? [{ label: t('app.newsletter.btn_delete_selected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDeleteCampaigns }]
			: [],
		headerActions: canManageNewsletter
			? [{ label: t('app.newsletter.btn_new_campaign', 'New Campaign'), icon: Plus, onclick: () => (showCreateModal = true) }]
			: [],
		load: async ({ page, pageSize, q, filter }) => {
			// Server-side pagination: send page_size + offset so the backend
			// returns the paginated envelope {items, total, total_pages}
			// instead of silently truncating at the default page size (§22).
			const params = new URLSearchParams({
				page_size: String(pageSize),
				offset: String(page * pageSize)
			});
			if (q) params.set('search', q);
			if (filter.campaign_status) params.set('status', filter.campaign_status);
			const result = await api<{ items: Campaign[]; total: number; total_pages: number } | Campaign[]>(
				`/admin/newsletter/campaigns?${params.toString()}`
			);
			if (result.ok) {
				const d = result.data;
				// Support both the paginated envelope and a plain array (fallback).
				const items = Array.isArray(d) ? d : (d?.items ?? []);
				const total = Array.isArray(d) ? d.length : (d?.total ?? 0);
				const totalPages = Array.isArray(d) ? 1 : (d?.total_pages ?? 1);
				// Compute derived open/click rates from the raw counters returned
				// by the backend (open_count/click_count/recipient_count). The
				// backend never returns a pre-computed rate — §62 schema.
				const data = items.map((c) => {
					const recipients = c.recipient_count ?? 0;
					const openRate = recipients > 0 ? ((c.open_count ?? 0) / recipients) * 100 : 0;
					const clickRate = recipients > 0 ? ((c.click_count ?? 0) / recipients) * 100 : 0;
					return { ...c, openRate, clickRate };
				});
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('app.newsletter.error_load_campaigns', 'Failed to load campaigns'));
		},
		viewSnippet: CampaignView
	});

	// ── Campaign send ───────────────────────────────────────────────────────
	async function sendCampaign(id: string) {
		pageError = '';
		try {
			const result = await api(`/admin/newsletter/campaigns/${id}/send`, { method: 'POST' });
			if (!result.ok) {
				pageError = result.error ?? t('app.newsletter.error_send_campaign', 'Failed to send campaign');
				return;
			}
			campaignController?.reload();
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('app.newsletter.error_send_campaign', 'Failed to send campaign');
		}
	}

	// ── Bulk delete campaigns (in-flight guard) ─────────────────────────────
	async function bulkDeleteCampaigns(ids: Set<string>) {
		if (ids.size === 0) return;
		if (bulkDeleting) return;
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(Array.from(ids).map((id) => api(`/admin/newsletter/campaigns/${id}`, { method: 'DELETE' })));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('app.newsletter.error_load_campaigns', 'Failed to delete campaigns');
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Create campaign (page-local Modal — sanitizes HTML) ─────────────────
	async function createCampaign() {
		if (!newCampaign.name || !newCampaign.subject) return;
		creating = true;
		pageError = '';
		// Use the hardened DOMParser-based sanitizeHtml (allowlisted tags and
		// attrs, dangerous schemes rejected, unsafe styles dropped) instead
		// of a regex strip — regexes miss entity-encoded and nested vectors.
		const sanitizedHtml = sanitizeHtml(newCampaign.html);
		try {
			const result = await api('/admin/newsletter/campaigns', { method: 'POST', body: { ...newCampaign, html: sanitizedHtml } });
			if (result.ok) {
				showCreateModal = false;
				newCampaign = { name: '', subject: '', html: '' };
				campaignController?.reload();
			} else {
				pageError = result.error ?? t('app.newsletter.error_create_campaign', 'Failed to create campaign');
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('app.newsletter.error_create_campaign', 'Failed to create campaign');
		} finally {
			creating = false;
		}
	}

	// ── Import subscribers (page-local Modal) ───────────────────────────────
	function openImportModal() {
		importText = '';
		importResult = null;
		showImportModal = true;
	}

	async function importSubscribers() {
		const lines = importText.split('\n').map((l) => l.trim()).filter((l) => l && l.includes('@'));
		if (lines.length === 0) return;
		importing = true;
		importResult = { success: 0, failed: 0, errors: [] };
		const emails = lines.map((line) => line.split(',')[0].trim()).filter((e) => e);
		try {
			const result = await api('/admin/newsletter/subscribers/import', { method: 'POST', body: { emails } });
			if (result.ok) {
				const data = (result.data ?? {}) as { imported?: number; skipped?: number };
				importResult.success = data.imported ?? 0;
				importResult.failed = data.skipped ?? 0;
			} else {
				importResult.failed = emails.length;
				importResult.errors.push(result.error ?? 'failed');
			}
		} catch (e) {
			importResult.failed = emails.length;
			importResult.errors.push(e instanceof Error ? e.message : 'failed');
		}
		importing = false;
		if (importResult.success > 0) subscriberController?.reload();
	}

	// ── Stats + analytics data ──────────────────────────────────────────────
	async function loadStats() {
		try {
			const result = await api<Array<{ status: string; count: number }>>('/admin/newsletter/stats');
			if (result.ok && result.data) {
				stats.subscribers = result.data.reduce((sum, s) => sum + s.count, 0);
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('app.newsletter.error_load_stats', 'Failed to load stats');
		}
	}

	async function loadAnalytics() {
		analyticsLoading = true;
		pageError = '';
		try {
			// Fetch all campaigns for the chart + average-rate computation.
			// Use a large page_size so the backend returns everything in one page.
			const params = new URLSearchParams({ page_size: '500', offset: '0' });
			const result = await api<{ items: Campaign[]; total: number; total_pages: number } | Campaign[]>(
				`/admin/newsletter/campaigns?${params.toString()}`
			);
			if (result.ok) {
				const d = result.data;
				const items = Array.isArray(d) ? d : (d?.items ?? []);
				const total = Array.isArray(d) ? d.length : (d?.total ?? 0);
				analyticsCampaigns = items.map((c) => {
					const recipients = c.recipient_count ?? 0;
					const openRate = recipients > 0 ? ((c.open_count ?? 0) / recipients) * 100 : 0;
					const clickRate = recipients > 0 ? ((c.click_count ?? 0) / recipients) * 100 : 0;
					return { ...c, openRate, clickRate };
				});
				stats.campaigns = total;
				// Average open/click rates are derived from the per-campaign
				// counters (open_count/click_count/recipient_count) — the backend
				// does not return pre-computed rates (§62 schema).
				const sentCampaigns = analyticsCampaigns.filter((c) => c.status === 'sent');
				if (sentCampaigns.length > 0) {
					stats.avgOpenRate = sentCampaigns.reduce((sum, c) => sum + (c.openRate ?? 0), 0) / sentCampaigns.length;
					stats.avgClickRate = sentCampaigns.reduce((sum, c) => sum + (c.clickRate ?? 0), 0) / sentCampaigns.length;
				}
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('app.newsletter.error_load_campaigns', 'Failed to load campaigns');
		} finally {
			analyticsLoading = false;
		}
	}

	// Load subscriber stats once on mount for the tab badge count.
	let initialized = false;
	$effect(() => {
		if (!initialized) {
			initialized = true;
			loadStats();
		}
	});

	// Refresh analytics data when the analytics tab is opened.
	$effect(() => {
		void activeTab;
		if (initialized && activeTab === 'analytics') {
			loadAnalytics();
		}
	});
</script>

<svelte:head>
	<title>{t('app.newsletter.page_title', 'Newsletter')} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-600 dark:text-red-400">{pageError}</div>
{/if}

<Tabs {tabs} bind:value={activeTab} tabParam="tab" tabPreserve={[]} />

{#if activeTab === 'subscribers'}
	<EntityPage config={subscriberConfig} bind:controller={subscriberController} />
{:else if activeTab === 'campaigns'}
	<EntityPage config={campaignConfig} bind:controller={campaignController} />
{:else if activeTab === 'analytics'}
	<div class="space-y-6">
		<div class="grid grid-cols-1 gap-6 md:grid-cols-3">
			<Card>
				<CardContent class="p-6">
					<p class="text-sm font-medium text-muted">{t('newsletter.total_subscribers')}</p>
					<p class="text-2xl font-bold text-foreground">{stats.subscribers}</p>
				</CardContent>
			</Card>
			<Card>
				<CardContent class="p-6">
					<p class="text-sm font-medium text-muted">{t('newsletter.avg_open_rate')}</p>
					<p class="text-2xl font-bold text-foreground">{stats.avgOpenRate}%</p>
				</CardContent>
			</Card>
			<Card>
				<CardContent class="p-6">
					<p class="text-sm font-medium text-muted">{t('newsletter.avg_click_rate')}</p>
					<p class="text-2xl font-bold text-foreground">{stats.avgClickRate}%</p>
				</CardContent>
			</Card>
		</div>
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold text-foreground">{t('newsletter.campaign_performance')}</h3>
			</CardHeader>
			<CardContent>
				{#if analyticsLoading}
					<div class="flex h-64 items-center justify-center rounded-lg bg-surface">
						<p class="text-muted">{t('common.loading', 'Loading…')}</p>
					</div>
				{:else if analyticsCampaigns.filter((c) => c.status === 'sent').length > 0}
					<div class="h-64">
						<BarChart
							data={analyticsCampaigns
								.filter((c) => c.status === 'sent')
								.map((c) => ({
									name: c.subject ?? c.name ?? 'Untitled',
									open_rate: c.openRate ?? 0,
									click_rate: c.clickRate ?? 0
								}))}
							x="name"
							y="open_rate"
							xPadding={[0.1, 0.1]}
							cRange={['var(--chart-1)']}
							axis={newsletterChartAxis}
						/>
					</div>
				{:else}
					<div class="flex h-64 items-center justify-center rounded-lg bg-surface">
						<p class="text-muted">{t('newsletter.analytics_chart_placeholder_integrate_wi')}</p>
					</div>
				{/if}
			</CardContent>
		</Card>
	</div>
{/if}

<!-- Create Campaign Modal (page-local — form with HTML textarea) -->
<Modal bind:open={showCreateModal} title={t('app.newsletter.modal_create_campaign', 'Create Campaign')} description={t('app.newsletter.modal_create_campaign_desc', 'Compose a new newsletter campaign')} size="lg">
	<div class="space-y-4">
		<div>
			<label for="campaign-name" class="mb-1 block text-sm font-medium text-foreground">{t('newsletter.campaign_name')}</label>
			<input id="campaign-name" type="text" class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder={t('app.newsletter.placeholder_campaign_name', 'e.g. Weekly Digest #42')} bind:value={newCampaign.name} />
		</div>
		<div>
			<label for="campaign-subject" class="mb-1 block text-sm font-medium text-foreground">{t('newsletter.subject_line')}</label>
			<input id="campaign-subject" type="text" class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder={t('app.newsletter.placeholder_subject', 'What subscribers will see in their inbox')} bind:value={newCampaign.subject} />
		</div>
		<div>
			<label for="campaign-html" class="mb-1 block text-sm font-medium text-foreground">{t('newsletter.content_html')}</label>
			<textarea id="campaign-html" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" rows="8" placeholder="<h1>Hello</h1><p>Your newsletter content here</p>" bind:value={newCampaign.html}></textarea>
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => (showCreateModal = false)}>{t('newsletter.cancel')}</Button>
			<Button onclick={createCampaign} disabled={creating || !newCampaign.name || !newCampaign.subject}>
				{creating ? t('app.newsletter.creating', 'Creating...') : t('app.newsletter.btn_create_campaign', 'Create Campaign')}
			</Button>
		</div>
	</div>
</Modal>

<!-- Import Subscribers Modal (page-local — form with textarea) -->
<Modal bind:open={showImportModal} title={t('app.newsletter.modal_import_subscribers', 'Import Subscribers')} size="lg">
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-muted)">
			{t('app.newsletter.import_hint', 'Paste email addresses below, one per line. Optionally add a name after a comma: email@example.com, John Doe')}
		</p>
		<textarea class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="10" bind:value={importText} placeholder={"user1@example.com\nuser2@example.com, Jane Doe\nuser3@example.com"}></textarea>
		{#if importResult}
			<div class="rounded-lg border p-3 text-sm" style="border-color: var(--color-border); background: var(--color-surface)">
				<p style="color: var(--color-success)">{t('app.newsletter.import_success', 'Imported')}: {importResult.success}</p>
				{#if importResult.failed > 0}
					<p style="color: var(--color-danger)">{t('app.newsletter.import_failed', 'Failed')}: {importResult.failed}</p>
					<div class="mt-2 max-h-32 overflow-y-auto">
						{#each importResult.errors as err (err)}
							<p class="text-xs" style="color: var(--color-muted)">{err}</p>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showImportModal = false)}>{t('common.close', 'Close')}</Button>
		<Button onclick={importSubscribers} disabled={importing || !importText.trim()}>
			{importing ? t('common.saving', 'Importing...') : t('app.newsletter.btn_import', 'Import')}
		</Button>
	</div>
</Modal>

<!-- ── View snippets ───────────────────────────────────────────────────── -->

{#snippet SubscriberView({ row }: { row: Subscriber })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.newsletter.col_email', 'Email')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.email}</p>
		</div>
		{#if row.name}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.newsletter.col_email', 'Name')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{row.name}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.newsletter.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.status}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.newsletter.col_subscribed', 'Subscribed')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{new Date(row.subscribedAt || row.created_at).toLocaleString()}</p>
		</div>
	</div>
{/snippet}

{#snippet CampaignView({ row }: { row: Campaign })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('newsletter.subject_line', 'Subject')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{row.subject || row.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('common.status', 'Status')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{row.status}</p>
			</div>
		</div>
		{#if row.preview}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('newsletter.content_html', 'Preview')}</p>
				<p class="mt-1 text-sm text-foreground">{row.preview}</p>
			</div>
		{/if}
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('newsletter.recipients', 'Recipients')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{row.recipient_count != null ? row.recipient_count.toLocaleString() : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.newsletter.col_open_rate', 'Open Rate')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{(row.openRate ?? 0).toFixed(1)}%</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('newsletter.avg_click_rate', 'Click Rate')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{(row.clickRate ?? 0).toFixed(1)}%</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.newsletter.col_sent', 'Sent')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{(row.sentAt || row.sent_at) ? new Date(row.sentAt || row.sent_at!).toLocaleString() : '—'}</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{new Date(row.created_at).toLocaleString()}</p>
		</div>
	</div>
{/snippet}
