<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Briefcase, RefreshCw } from 'lucide-svelte';
	import { api } from '$lib/api';

	type JobApplication = {
		id: string;
		job_id: string;
		job_title: string;
		job_slug: string;
		applicant_id: string;
		applicant_username: string;
		applicant_email: string;
		status: string;
		employer_notes: string | null;
		applied_at: string;
		status_updated_at: string;
	};

	type StatusDist = Record<string, number>;

	const canManage = $derived(hasPermission('jobs.admin'));

	let applications = $state<JobApplication[]>([]);
	let statusDist = $state<StatusDist>({});
	let loading = $state(true);
	let error = $state('');
	let message = $state('');
	let messageType = $state('');
	let togglingId = $state('');

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await api<{ data: JobApplication[]; status_distribution: StatusDist }>('/admin/jobs');
			if (result.ok) {
				const raw = result.data as { data?: JobApplication[]; status_distribution?: StatusDist } | JobApplication[];
				if (Array.isArray(raw)) {
					applications = raw;
				} else {
					applications = raw.data ?? [];
					statusDist = raw.status_distribution ?? {};
				}
			} else {
				error = result.error;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('jobs.load_error', 'Failed to load job applications');
		} finally {
			loading = false;
		}
	}

	async function toggleFeature(id: string) {
		togglingId = id;
		message = '';
		try {
			const result = await api(`/admin/jobs/${id}/feature`, { method: 'PATCH' });
			if (result.ok) {
				messageType = 'success';
				message = t('jobs.feature_toggled', 'Featured status toggled.');
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('jobs.toggle_error', 'Failed to toggle feature');
		} finally {
			togglingId = '';
		}
	}

	const columns = $derived([
		{
			key: 'job_title',
			label: t('jobs.title', 'Job Title'),
			sortable: true,
			render: (value: string) => `<span class="font-medium text-foreground">${value ?? '—'}</span>`
		},
		{
			key: 'applicant_username',
			label: t('jobs.applicant', 'Applicant'),
			sortable: true,
			render: (value: string) => `<span class="text-foreground">${value ?? '—'}</span>`
		},
		{
			key: 'applicant_email',
			label: t('jobs.email', 'Email'),
			render: (value: string) => `<span class="text-muted-foreground text-xs">${value ?? '—'}</span>`
		},
		{
			key: 'status',
			label: t('jobs.status', 'Status'),
			sortable: true,
			render: (value: string) => {
				const clsMap: Record<string, string> = {
					pending: 'bg-warning/15 text-warning border-warning',
					accepted: 'bg-success/15 text-success border-success',
					rejected: 'bg-danger/15 text-danger border-danger',
					interviewing: 'bg-primary/15 text-primary border-primary'
				};
				const cls = clsMap[value] ?? 'bg-muted text-muted-foreground border-border';
				return `<span class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${cls}">${value}</span>`;
			}
		},
		{
			key: 'applied_at',
			label: t('jobs.applied', 'Applied'),
			sortable: true,
			render: (value: string) => `<span class="text-muted-foreground text-xs">${value ? new Date(value).toLocaleDateString() : '—'}</span>`
		}
	]);

	load();
</script>

<PageHeader
	title={t('jobs.title', 'Jobs Board')}
	description={t('jobs.description', 'Manage job applications and featured listings.')}
/>

{#if message}
	<div
		class="mb-4 rounded-lg border p-3 text-sm"
		style={messageType === 'success'
			? 'border-color: var(--color-success); background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)'
			: 'border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)'}
	>
		{message}
	</div>
{/if}

{#if loading}
	<div class="flex items-center gap-2 p-8" style="color: var(--color-muted)">
		<RefreshCw class="h-4 w-4 animate-spin" />
		<span>{t('jobs.loading', 'Loading applications...')}</span>
	</div>
{:else if error}
	<div class="rounded-lg border p-4" style="border-color: var(--color-danger); color: var(--color-danger)">
		{error}
	</div>
{:else}
	{#if Object.keys(statusDist).length > 0}
		<div class="mb-6 grid gap-3 sm:grid-cols-4">
			{#each Object.entries(statusDist) as [status, count]}
				<Card>
					<CardHeader>
						<span class="text-xs font-medium" style="color: var(--color-muted)">{status}</span>
					</CardHeader>
					<CardContent>
						<p class="text-2xl font-bold" style="color: var(--color-foreground)">{count}</p>
					</CardContent>
				</Card>
			{/each}
		</div>
	{/if}

	<DataTable
		data={applications}
		{columns}
		emptyMessage={t('jobs.empty', 'No job applications found.')}
	/>
{/if}
