<script lang="ts">
	// /mentorship — migrated to the EntityPage scaffold.
	// Two tabs: Programs (full CRUD) and Matches (view + status update).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { mentorshipAdminApiExtended as mentorshipApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Tabs } from '$lib/components/ui/tabs';
	import { Trash2, CheckCircle, XCircle, RotateCcw } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type Program = {
		id: string;
		name: string;
		description?: string;
		is_active: boolean;
		mentor_count: number;
		active_mentee_count: number;
		match_count: number;
		created_at: string;
	};

	type Match = {
		id: string;
		program_id: string;
		program_name?: string;
		mentor_id: string;
		mentor_username?: string;
		mentee_id: string;
		mentee_username?: string;
		status: string;
		created_at: string;
		updated_at: string;
	};

	// §102 Mentorship Matching (ADR-110) — connection type.
	type Connection = {
		id: string;
		mentor_id: string;
		mentor_username?: string;
		mentee_id: string;
		mentee_username?: string;
		status: string;
		goals?: string | null;
		created_at: string;
		updated_at: string;
	};

	const MATCH_STATUSES = ['active', 'completed', 'cancelled'] as const;
	const CONNECTION_STATUSES = ['pending', 'active', 'completed', 'cancelled', 'declined'] as const;

	let activeTab = $state('programs');
	const tabs = [
		{ value: 'programs', label: t('app.mentorship.tabs.programs', 'Programs') },
		{ value: 'matches', label: t('app.mentorship.tabs.matches', 'Matches') },
		{ value: 'connections', label: t('app.mentorship.tabs.connections', 'Connections') }
	];

	const canManageMentorship = hasPermission('mentorship.admin');

	// ── Programs EntityPage ──────────────────────────────────────────────

	let programController = $state<EntityPageController | undefined>(undefined);
	let programPageError = $state('');
	let programBulkDeleting = $state(false);

	async function bulkDeletePrograms(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (programBulkDeleting) return;
		programBulkDeleting = true;
		programPageError = '';
		try {
			await Promise.all(idArr.map((id) => mentorshipApi.deleteProgram(id)));
		} catch (e) {
			programPageError = e instanceof Error ? e.message : t('app.mentorship.error.delete_failed', 'Failed to delete programs');
		} finally {
			programBulkDeleting = false;
		}
	}

	const programFields: FieldDef[] = [
		{ key: 'name', label: t('app.mentorship.label.program_name', 'Program Name'), type: 'text', placeholder: t('app.mentorship.placeholder.name', 'e.g. Career Growth Mentorship'), required: true },
		{ key: 'description', label: t('app.mentorship.label.description', 'Description'), type: 'textarea', placeholder: t('app.mentorship.placeholder.description', 'What is this program about?') },
		{ key: 'is_active', label: t('app.mentorship.status.active', 'Active'), type: 'checkbox' }
	];

	const programConfig: EntityPageConfig = {
		title: t('app.mentorship.title', 'Mentorship'),
		description: t('app.mentorship.description', 'Manage mentorship programs and matching'),
		createLabel: t('app.mentorship.actions.new_program', 'New Program'),
		createPermission: 'mentorship.admin',
		pageSize: 15,
		columns: [
			{
				key: 'name',
				label: t('app.mentorship.columns.program', 'Program'),
				sortable: true,
				render: (_: any, row: Program) =>
					`<div><p class="font-medium text-foreground">${row.name}</p>${row.description ? `<p class="text-sm text-muted line-clamp-1">${row.description}</p>` : ''}</div>`
			},
			{
				key: 'mentor_count',
				label: t('app.mentorship.columns.mentors', 'Mentors'),
				sortable: true,
				render: (v: number) => `<span class="font-medium">${v ?? 0}</span>`
			},
			{
				key: 'active_mentee_count',
				label: t('app.mentorship.columns.mentees', 'Mentees'),
				sortable: true,
				render: (v: number) => `<span class="font-medium">${v ?? 0}</span>`
			},
			{
				key: 'match_count',
				label: t('app.mentorship.columns.matches', 'Matches'),
				sortable: true,
				render: (v: number) => `<span class="text-sm text-foreground-alt">${v ?? '—'}</span>`
			},
			{
				key: 'is_active',
				label: t('app.mentorship.columns.status', 'Status'),
				sortable: true,
				render: (v: boolean) => {
					const classes = v ? 'bg-success/15 text-success border-success' : 'bg-surface text-foreground border';
					const label = v ? t('app.mentorship.status.active', 'Active') : t('app.mentorship.status.inactive', 'Inactive');
					return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes}">${label}</span>`;
				}
			},
			{
				key: 'created_at',
				label: t('app.mentorship.columns.created', 'Created'),
				sortable: true,
				render: (v: string) => new Date(v).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' })
			}
		],
		filters: [
			{
				key: 'is_active',
				label: t('app.mentorship.columns.status', 'Status'),
				options: [
					{ value: 'true', label: t('app.mentorship.status.active', 'Active') },
					{ value: 'false', label: t('app.mentorship.status.inactive', 'Inactive') }
				]
			}
		],
		bulkActions: [
			{
				label: t('app.mentorship.actions.delete_selected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDeletePrograms
			}
		],
		fields: programFields,
		load: async ({ q, filter }) => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			if (filter.is_active) params.set('is_active', filter.is_active);
			const result = await mentorshipApi.listPrograms(params.toString() || undefined);
			if (result.ok) {
				const data = (result.data as Program[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('app.mentorship.error.load_failed', 'Failed to load programs'));
		},
		create: async (body) => {
			const res = await mentorshipApi.createProgram({
				name: String(body.name ?? '').trim(),
				description: String(body.description ?? '').trim() || undefined,
				is_active: body.is_active === undefined ? true : !!body.is_active
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await mentorshipApi.updateProgram(id, {
				name: String(body.name ?? '').trim(),
				description: String(body.description ?? '').trim() || undefined,
				is_active: !!body.is_active
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await mentorshipApi.deleteProgram(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: ProgramView
	};

	// ── Matches EntityPage ───────────────────────────────────────────────

	let matchController = $state<EntityPageController | undefined>(undefined);
	let matchPageError = $state('');

	async function updateMatchStatus(id: string, status: string) {
		matchPageError = '';
		try {
			const res = await mentorshipApi.updateMatch(id, { status });
			if (!res.ok) {
				matchPageError = res.error ?? t('app.mentorship.error.update_match_failed', 'Failed to update match');
			}
		} catch (e) {
			matchPageError = e instanceof Error ? e.message : t('app.mentorship.error.update_match_failed', 'Failed to update match');
		}
		matchController?.reload();
	}

	const matchActions: ActionItem[] = [
		{
			label: t('app.mentorship.match.status_active', 'Mark Active'),
			icon: CheckCircle,
			onclick: (row: Match) => updateMatchStatus(row.id, 'active'),
			show: (row: Match) => row.status !== 'active'
		},
		{
			label: t('app.mentorship.match.status_completed', 'Mark Completed'),
			icon: CheckCircle,
			onclick: (row: Match) => updateMatchStatus(row.id, 'completed'),
			show: (row: Match) => row.status !== 'completed'
		},
		{
			label: t('app.mentorship.match.status_cancelled', 'Cancel Match'),
			icon: XCircle,
			variant: 'destructive',
			onclick: (row: Match) => updateMatchStatus(row.id, 'cancelled'),
			show: (row: Match) => row.status !== 'cancelled'
		}
	];

	const matchConfig: EntityPageConfig = {
		title: t('app.mentorship.tabs.matches', 'Matches'),
		description: t('app.mentorship.matches_description', 'View and manage mentor-mentee matches'),
		createPermission: 'mentorship.admin',
		readonly: true,
		pageSize: 15,
		columns: [
			{
				key: 'program_name',
				label: t('app.mentorship.columns.program', 'Program'),
				sortable: true,
				render: (v: string) => v ? `<span class="font-medium text-foreground">${v}</span>` : `<span class="text-muted">—</span>`
			},
			{
				key: 'mentor_username',
				label: t('app.mentorship.columns.mentor', 'Mentor'),
				sortable: true,
				render: (v: string) => v ? `<span class="font-medium text-foreground">@${v}</span>` : `<span class="text-muted">—</span>`
			},
			{
				key: 'mentee_username',
				label: t('app.mentorship.columns.mentee', 'Mentee'),
				sortable: true,
				render: (v: string) => v ? `<span class="font-medium text-foreground">@${v}</span>` : `<span class="text-muted">—</span>`
			},
			{
				key: 'status',
				label: t('app.mentorship.columns.status', 'Status'),
				sortable: true,
				render: (v: string) => {
					const map: Record<string, string> = {
						active: 'bg-success/15 text-success border-success',
						completed: 'bg-primary/15 text-primary border-primary',
						cancelled: 'bg-danger/15 text-danger border-danger'
					};
					const classes = map[v] ?? 'bg-surface text-foreground border';
					return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes}">${v}</span>`;
				}
			},
			{
				key: 'created_at',
				label: t('app.mentorship.columns.created', 'Created'),
				sortable: true,
				render: (v: string) => new Date(v).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' })
			}
		],
		filters: [
			{
				key: 'status',
				label: t('app.mentorship.columns.status', 'Status'),
				options: MATCH_STATUSES.map((s) => ({ value: s, label: s.charAt(0).toUpperCase() + s.slice(1) }))
			}
		],
		actions: matchActions,
		fields: [],
		load: async ({ q, filter }) => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			if (filter.status) params.set('status', filter.status);
			const result = await mentorshipApi.listMatches(params.toString() || undefined);
			if (result.ok) {
				const data = (result.data as Match[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('app.mentorship.error.load_matches_failed', 'Failed to load matches'));
		},
		viewSnippet: MatchView
	};

	// ── Connections EntityPage (§102 ADR-110) ────────────────────────────

	let connectionController = $state<EntityPageController | undefined>(undefined);
	let connectionPageError = $state('');

	const connectionFields: FieldDef[] = [
		{
			key: 'status',
			label: t('app.mentorship.columns.status', 'Status'),
			type: 'select',
			options: CONNECTION_STATUSES.map((s) => ({ value: s, label: s }))
		}
	];

	const connectionConfig: EntityPageConfig = {
		title: t('app.mentorship.tabs.connections', 'Connections'),
		description: t(
			'app.mentorship.connections_description',
			'View and override mentorship connections (§102)'
		),
		createPermission: 'mentorship.manage',
		editPermission: 'mentorship.manage',
		readonly: !hasPermission('mentorship.manage'),
		pageSize: 15,
		columns: [
			{
				key: 'mentor_username',
				label: t('app.mentorship.columns.mentor', 'Mentor'),
				sortable: true,
				render: (_: any, row: Connection) =>
					`<span class="font-medium text-foreground">${row.mentor_username ? '@' + row.mentor_username : row.mentor_id}</span>`
			},
			{
				key: 'mentee_username',
				label: t('app.mentorship.columns.mentee', 'Mentee'),
				sortable: true,
				render: (_: any, row: Connection) =>
					`<span class="font-medium text-foreground">${row.mentee_username ? '@' + row.mentee_username : row.mentee_id}</span>`
			},
			{
				key: 'status',
				label: t('app.mentorship.columns.status', 'Status'),
				sortable: true,
				render: (v: string) => `<span class="font-medium">${v}</span>`
			},
			{
				key: 'created_at',
				label: t('app.mentorship.columns.created', 'Created'),
				sortable: true,
				render: (v: string) => `<span class="text-muted">${new Date(v).toLocaleDateString()}</span>`
			}
		],
		filters: [
			{
				key: 'status',
				label: t('app.mentorship.columns.status', 'Status'),
				options: CONNECTION_STATUSES.map((s) => ({ value: s, label: s }))
			}
		],
		fields: connectionFields,
		load: async ({ q, sort, filter }) => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			if (sort) {
				params.set('sort', sort);
			}
			if (filter.status) params.set('status', filter.status);
			const result = await mentorshipApi.listConnections(params.toString() || undefined);
			if (result.ok) {
				const data = (result.data as Connection[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(
				result.error ?? t('app.mentorship.error.load_connections_failed', 'Failed to load connections')
			);
		},
		update: async (id, body) => {
			const result = await mentorshipApi.updateConnection(id, body);
			if (result.ok) return { ok: true };
			return { ok: false, error: result.error ?? 'Failed to update connection' };
		},
		viewSnippet: ConnectionView
	};
</script>

<svelte:head>
	<title>{t('app.mentorship.page_title', 'Mentorship — JapaConnect Admin')}</title>
</svelte:head>

<Tabs {tabs} bind:value={activeTab} />

{#if activeTab === 'programs'}
	{#if programPageError}
		<div class="mb-4 rounded-lg border border-danger/30 bg-danger/10 p-3 text-sm text-danger">
			{programPageError}
		</div>
	{/if}
	<EntityPage config={programConfig} bind:controller={programController} />
{:else if activeTab === 'matches'}
	{#if matchPageError}
		<div class="mb-4 rounded-lg border border-danger/30 bg-danger/10 p-3 text-sm text-danger">
			{matchPageError}
		</div>
	{/if}
	<EntityPage config={matchConfig} bind:controller={matchController} />
{:else if activeTab === 'connections'}
	{#if connectionPageError}
		<div class="mb-4 rounded-lg border border-danger/30 bg-danger/10 p-3 text-sm text-danger">
			{connectionPageError}
		</div>
	{/if}
	<EntityPage config={connectionConfig} bind:controller={connectionController} />
{/if}

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet ProgramView({ row }: { row: Program })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.field.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.name}</p>
		</div>
		{#if row.description}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.field.description', 'Description')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{row.description}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.field.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.is_active ? t('app.mentorship.status.active', 'Active') : t('app.mentorship.status.inactive', 'Inactive')}</p>
		</div>
		<div class="grid grid-cols-3 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.field.mentors', 'Mentors')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{row.mentor_count ?? 0}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.field.mentees', 'Mentees')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{row.active_mentee_count ?? 0}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.matches', 'Matches')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{row.match_count ?? 0}</p>
			</div>
		</div>
	</div>
{/snippet}

{#snippet MatchView({ row }: { row: Match })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.program', 'Program')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.program_name ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.mentor', 'Mentor')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.mentor_username ? `@${row.mentor_username}` : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.mentee', 'Mentee')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.mentee_username ? `@${row.mentee_username}` : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.status}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{new Date(row.created_at).toLocaleString()}</p>
		</div>
	</div>
{/snippet}

{#snippet ConnectionView({ row }: { row: Connection })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.mentor', 'Mentor')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.mentor_username ? `@${row.mentor_username}` : row.mentor_id}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.mentee', 'Mentee')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.mentee_username ? `@${row.mentee_username}` : row.mentee_id}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.status}</p>
		</div>
		{#if row.goals}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.field.goals', 'Goals')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{row.goals}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{new Date(row.created_at).toLocaleString()}</p>
		</div>
	</div>
{/snippet}
