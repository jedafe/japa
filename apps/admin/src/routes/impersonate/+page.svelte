<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { impersonationApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	// Ref: japaconnect-engineering/sections/46-impersonation.md — every
	// impersonation start/end must write to audit_logs (Rule 10). The backend
	// handles the audit-log write; this page only calls the API endpoints.
	import { ShieldCheck, ShieldAlert, LogOut, RefreshCw, UserSearch } from 'lucide-svelte';

	type ImpersonationStatus = {
		active: boolean;
		target_user_id: string | null;
		expires_at: string | null;
		target: { id: string; username: string; email: string } | null;
	};

	let status = $state<ImpersonationStatus>({ active: false, target_user_id: null, expires_at: null, target: null });
	let loading = $state(true);
	let ending = $state(false);
	let showEndConfirm = $state(false);
	let error = $state('');
	let success = $state('');

	const canImpersonate = $derived(hasPermission('admin.impersonate'));

	async function loadStatus() {
		loading = true;
		error = '';
		try {
			const result = await impersonationApi.status();
			if (result.ok) {
				status = result.data;
			} else {
				error = t('impersonate.load_error', 'Failed to load impersonation status');
			}
		} catch {
			error = t('impersonate.load_error', 'Failed to load impersonation status');
		} finally {
			loading = false;
		}
	}

	async function endImpersonation() {
		showEndConfirm = false;
		ending = true;
		error = '';
		try {
			const result = await impersonationApi.end();
			if (result.ok) {
				success = t('impersonate.ended', 'Impersonation ended');
				status = { active: false, target_user_id: null, expires_at: null, target: null };
				setTimeout(() => (success = ''), 3000);
			} else {
				error = t('impersonate.end_error', 'Failed to end impersonation');
			}
		} catch {
			error = t('impersonate.end_error', 'Failed to end impersonation');
		} finally {
			ending = false;
		}
	}

	onMount(() => {
		loadStatus();
	});
</script>

<PageHeader
	title={t('impersonate.title', 'Impersonation')}
	description={t('impersonate.description', 'View and manage active impersonation sessions')}
/>

{#if error}
	<div class="mb-4 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-600 dark:text-red-400">
		{error}
	</div>
{/if}
{#if success}
	<div class="mb-4 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-400">
		{success}
	</div>
{/if}

{#if !canImpersonate}
	<Card>
		<CardContent class="py-12 text-center">
			<ShieldAlert class="mx-auto mb-3 h-10 w-10 text-muted-foreground" />
			<p class="text-muted-foreground">{t('impersonate.no_permission', 'You do not have permission to manage impersonation.')}</p>
		</CardContent>
	</Card>
{:else if loading}
	<Card>
		<CardContent class="py-12 text-center">
			<RefreshCw class="mx-auto mb-3 h-8 w-8 animate-spin text-muted-foreground" />
			<p class="text-muted-foreground">{t('common.loading', 'Loading...')}</p>
		</CardContent>
	</Card>
{:else}
	<div class="grid gap-4 sm:grid-cols-2">
		<!-- Active session status -->
		<Card>
			<CardHeader>
				<div class="flex items-center justify-between">
					<h2 class="text-lg font-semibold">{t('impersonate.current_session', 'Current Session')}</h2>
					<Button variant="outline" size="sm" onclick={loadStatus} disabled={loading}>
						<RefreshCw class="me-1.5 h-3.5 w-3.5" />
						{t('common.refresh', 'Refresh')}
					</Button>
				</div>
			</CardHeader>
			<CardContent>
				{#if status.active}
					<div class="flex items-center gap-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-4">
						<ShieldAlert class="h-8 w-8 text-amber-600 dark:text-amber-400" />
						<div class="flex-1">
							<p class="text-sm font-medium text-amber-700 dark:text-amber-300">
								{t('impersonate.active', 'Impersonating')}
							</p>
							<p class="text-sm text-muted-foreground">
								{t('impersonate.impersonating_user', 'You are currently impersonating {username}').replace('{username}', status.target?.username ?? status.target_user_id ?? '—')}
							</p>
							{#if status.target?.email}
								<p class="mt-1 text-xs text-muted-foreground">{status.target.email}</p>
							{/if}
							{#if status.target_user_id}
								<p class="mt-1 font-mono text-xs text-muted-foreground">{status.target_user_id}</p>
							{/if}
							{#if status.expires_at}
								<p class="mt-1 text-xs text-muted-foreground">{t("impersonate.expires_at", "Expires at")}: {status.expires_at}</p>
							{/if}
						</div>
						<Badge variant="warning">{t('impersonate.active_badge', 'Active')}</Badge>
					</div>
					<div class="mt-4">
						<Button variant="destructive" onclick={() => (showEndConfirm = true)} disabled={ending}>
							<LogOut class="me-1.5 h-4 w-4 rtl:-scale-x-100" />
							{t('impersonate.end', 'End Impersonation')}
						</Button>
					</div>
				{:else}
					<div class="flex items-center gap-3 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-4">
						<ShieldCheck class="h-8 w-8 text-emerald-600 dark:text-emerald-400" />
						<div class="flex-1">
							<p class="text-sm font-medium text-emerald-700 dark:text-emerald-300">
								{t('impersonate.not_active', 'No active impersonation')}
							</p>
							<p class="text-sm text-muted-foreground">
								{t('impersonate.not_active_desc', 'You are operating as yourself.')}
							</p>
						</div>
						<Badge variant="success">{t('impersonate.inactive_badge', 'Inactive')}</Badge>
					</div>
				{/if}
			</CardContent>
		</Card>

		<!-- How to impersonate -->
		<Card>
			<CardHeader>
				<h2 class="text-lg font-semibold">{t('impersonate.how_to_title', 'How to Impersonate')}</h2>
			</CardHeader>
			<CardContent>
				<div class="space-y-3 text-sm text-muted-foreground">
					<div class="flex items-start gap-2.5">
						<UserSearch class="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
						<p>{t('impersonate.step_1', 'Navigate to a user\'s detail page from the Users list.')}</p>
					</div>
					<div class="flex items-start gap-2.5">
						<ShieldCheck class="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
						<p>{t('impersonate.step_2', 'Click the "Impersonate" action in the user detail page.')}</p>
					</div>
					<div class="flex items-start gap-2.5">
						<LogOut class="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground rtl:-scale-x-100" />
						<p>{t('impersonate.step_3', 'Return here or use the banner to end the impersonation session when done.')}</p>
					</div>
				</div>
				<div class="mt-4">
					<Button variant="outline" onclick={() => goto('/users')}>
						<UserSearch class="me-1.5 h-4 w-4" />
						{t('impersonate.go_to_users', 'Go to Users')}
					</Button>
				</div>
			</CardContent>
		</Card>
	</div>

	<!-- Audit notice -->
	<Card class="mt-4">
		<CardContent class="py-4">
			<div class="flex items-start gap-2.5 text-sm text-muted-foreground">
				<ShieldAlert class="mt-0.5 h-4 w-4 shrink-0" />
				<p>
					{t('impersonate.audit_notice', 'All impersonation start and end actions are recorded in the audit log for compliance (Section 46).')}
				</p>
			</div>
		</CardContent>
	</Card>
{/if}

<Modal bind:open={showEndConfirm} title={t('impersonate.end_confirm_title', 'End Impersonation')}>
	<p class="text-sm text-muted-foreground">
		{t('impersonate.end_confirm', 'Are you sure you want to end the current impersonation session? You will return to your own account.')}
	</p>
	<div class="mt-6 flex justify-end gap-3">
		<Button variant="outline" onclick={() => (showEndConfirm = false)}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={endImpersonation} disabled={ending}>
			<LogOut class="me-1.5 h-4 w-4 rtl:-scale-x-100" />
			{t('impersonate.end', 'End Impersonation')}
		</Button>
	</div>
</Modal>
