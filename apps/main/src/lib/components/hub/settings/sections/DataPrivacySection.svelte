<script lang="ts">
	/**
	 * Data & Privacy settings — Phase 18.11 (GDPR/NDPR compliance).
	 *
	 * Three surfaces:
	 *  1. Consent toggles — GET/POST/DELETE /consent (per consent type).
	 *  2. Data export — POST /me/data-export (request) + GET /me/data-export/status (poll).
	 *  3. Account deactivation / deletion — POST /me/deactivate, DELETE /me.
	 *
	 * The DangerSection already covers deactivation/deletion with a confirm
	 * dialog; this section focuses on consent + export and links to /settings/danger
	 * for the destructive actions (avoiding duplication).
	 *
	 * Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
	 *      japaconnect-engineering/sections/50-user-dashboard-settings.md
	 */
	import { untrack, onMount } from 'svelte';
	import { Switch } from 'bits-ui';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { exportData } from '$lib/api/hub';
	import { Download, Loader2, ShieldCheck } from 'lucide-svelte';

	interface ConsentRecord {
		type: string;
		consented: boolean;
		consented_at?: string | null;
	}

	interface DataPrivacyData {
		consents: ConsentRecord[];
		exportStatus: { status: 'none' | 'pending' | 'ready' | 'failed'; download_url?: string | null };
	}

	let { data }: { data: unknown } = $props();
	const payload = $derived(data as DataPrivacyData | null);
	const consents = $state<ConsentRecord[]>(untrack(() => payload?.consents ?? []));
	const exportStatus = $state<{ status: string; download_url?: string | null }>(
		untrack(() => payload?.exportStatus ?? { status: 'none' })
	);

	let requesting = $state(false);
	let requestError = $state('');
	let requestSuccess = $state('');

	// Re-seed on SSR payload change.
	$effect(() => {
		const p = data as DataPrivacyData | null;
		consents.splice(0, consents.length, ...(p?.consents ?? []));
		Object.assign(exportStatus, p?.exportStatus ?? { status: 'none' });
	});

	/** Toggle a consent type on/off via POST /consent or DELETE /consent/{type}. */
	async function toggleConsent(idx: number) {
		const c = consents[idx];
		if (!c) return;
		const wasConsented = c.consented;
		// Optimistic
		consents[idx] = { ...c, consented: !wasConsented };
		try {
			if (wasConsented) {
				await api.delete(`/consent/${c.type}`);
			} else {
				await api.post('/consent', { type: c.type });
			}
		} catch {
			// Revert on error
			consents[idx] = c;
		}
	}

	/** Request a data export — POST /me/data-export. */
	async function requestExport() {
		if (requesting) return;
		requesting = true;
		requestError = '';
		requestSuccess = '';
		const res = await exportData();
		if (res.error) {
			requestError = t('settings.data_privacy.export_error', 'Failed to request export. Try again later.');
		} else {
			requestSuccess = t('settings.data_privacy.export_requested', 'Export requested. You will be emailed when it is ready.');
			exportStatus.status = 'pending';
		}
		requesting = false;
	}

	// Poll export status while pending (every 30s).
	onMount(() => {
		if (exportStatus.status !== 'pending') return;
		const interval = setInterval(async () => {
			if (exportStatus.status !== 'pending') {
				clearInterval(interval);
				return;
			}
			const res = await api.get<{ status: string; download_url?: string | null }>('/me/data-export/status');
			if (res.data) {
				exportStatus.status = res.data.status;
				exportStatus.download_url = res.data.download_url ?? null;
				if (res.data.status !== 'pending') clearInterval(interval);
			}
		}, 30000);
		return () => clearInterval(interval);
	});
</script>

<section class="max-w-2xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.data_privacy')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.data_privacy.desc', 'Manage your data, consent preferences, and GDPR/NDPR rights.')}</p>
	</header>

	<!-- Consent toggles -->
	<section class="mb-6">
		<h2 class="mb-3 flex items-center gap-1.5 text-sm font-medium text-[var(--color-text)]">
			<ShieldCheck size={15} /> {t('settings.data_privacy.consents', 'Consent preferences')}
		</h2>
		{#if consents.length === 0}
			<p class="text-sm text-[var(--color-muted)]">{t('settings.data_privacy.no_consents', 'No consent records yet.')}</p>
		{:else}
			<ul class="space-y-2">
				{#each consents as c, idx (c.type)}
					<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2">
						<span class="min-w-0 flex-1">
							<span class="block text-sm font-medium text-[var(--color-text)]">{t(`consent.${c.type}.title`, c.type)}</span>
							<span class="block text-xs text-[var(--color-muted)]">{t(`consent.${c.type}.desc`, '')}</span>
						</span>
							<Switch.Root
								checked={c.consented}
								onCheckedChange={() => toggleConsent(idx)}
								class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors data-[state=checked]:bg-[var(--color-primary)] data-[state=unchecked]:bg-[var(--color-border)]"
							>
								<Switch.Thumb
									class="inline-block h-4 w-4 transform rounded-full bg-white transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0.5"
								/>
							</Switch.Root>
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	<!-- Data export -->
	<section class="mb-6">
		<h2 class="mb-3 flex items-center gap-1.5 text-sm font-medium text-[var(--color-text)]">
			<Download size={15} /> {t('settings.data_privacy.data_export', 'Download your data')}
		</h2>
		<p class="mb-3 text-sm text-[var(--color-muted)]">
			{t('settings.data_privacy.export_desc', 'Request a copy of all data we hold about you. We will email you a download link when it is ready.')}
		</p>

		{#if requestError}
			<p class="mb-3 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">{requestError}</p>
		{/if}
		{#if requestSuccess}
			<p class="mb-3 rounded-[var(--radius)] bg-green-500/10 px-3 py-2 text-sm text-green-600" role="status">{requestSuccess}</p>
		{/if}

		{#if exportStatus.status === 'ready' && exportStatus.download_url}
			<a
				href={exportStatus.download_url}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
			>
				<Download size={14} /> {t('settings.data_privacy.download_ready', 'Download ready')}
			</a>
		{:else if exportStatus.status === 'pending'}
			<p class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
				<Loader2 size={14} class="animate-spin" />
				{t('settings.data_privacy.export_pending', 'Export in progress...')}
			</p>
		{:else}
			<button
				type="button"
				onclick={requestExport}
				disabled={requesting}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
			>
				{#if requesting}
					<Loader2 size={14} class="animate-spin" />
				{:else}
					<Download size={14} />
				{/if}
				{t('settings.data_privacy.request_export', 'Request data export')}
			</button>
		{/if}
	</section>

	<!-- Deactivation / deletion — link to Danger section -->
	<section>
		<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">
			{t('settings.data_privacy.danger_zone', 'Account closure')}
		</h2>
		<p class="text-sm text-[var(--color-muted)]">
			{t('settings.data_privacy.danger_desc', 'Deactivate or permanently delete your account from the Danger Zone.')}
		</p>
		<a
			href="/settings/danger"
			class="mt-2 inline-flex items-center gap-1 text-sm font-medium text-red-600 hover:underline"
		>
			{t('settings.data_privacy.go_to_danger', 'Go to Danger Zone →')}
		</a>
	</section>
</section>
