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
	import { api, apiBaseUrl } from '$lib/api/client';
	import { buildApiUrl } from '$lib/api/url';
	import { exportData, cancelDataExport } from '$lib/api/hub';
	import { Download, Loader2, ShieldCheck, X } from 'lucide-svelte';

	/** Backend consent record — matches gen.ConsentRecord (Section 19). */
	interface ConsentRecord {
		consent_type: string;
		granted: boolean;
		policy_version?: string;
		created_at?: string | null;
	}

	// Consent types and policy version come from GET /consent/config
	// (loaded server-side and refreshed client-side). No hardcoded defaults
	// are kept per Rule 16; the UI renders from the API or shows an empty
	// state while the client retries.
	let consentTypes = $state<string[]>([]);
	let policyVersion = $state('');

	interface DataPrivacyData {
		consents: ConsentRecord[];
		consentConfig?: { consent_types: string[]; policy_version: string } | null;
		exportStatus: ExportStatusResponse;
	}

	/** Raw response from GET /me/data-export/status. */
	type ExportStatusResponse =
		| { status: 'no_pending_export' }
		| { status: string; download_url?: string | null; [k: string]: unknown };

	/** Normalized status used by the UI. */
	type UiStatus = 'none' | 'pending' | 'ready' | 'failed';

	let { data }: { data: unknown } = $props();
	const payload = $derived(data as DataPrivacyData | null);

	/** Build a map of consent_type → granted from the API response.
	 *  All consent types default to true (opt-out model) — the user
	 *  must explicitly revoke a consent to turn it off. Existing API
	 *  records override the default. */
	function buildConsentMap(records: ConsentRecord[]): Map<string, boolean> {
		const map = new Map<string, boolean>();
		// Pre-seed all consent types as granted by default.
		for (const type of consentTypes) map.set(type, true);
		const seen = new Set<string>();
		for (const r of records) {
			// Only the latest record per type matters; backend returns newest-first.
			if (seen.has(r.consent_type)) continue;
			seen.add(r.consent_type);
			map.set(r.consent_type, r.granted);
		}
		return map;
	}

	/** Normalize the raw export status response into UI states.
	 *  Backend statuses: pending, processing, completed, failed, cancelled.
	 *  - cancelled: the user (or admin) aborted the export — treat as 'none'
	 *    so the user can request a new one (otherwise the UI is stuck in
	 *    'pending' forever with an infinite polling loop). */
	function normalizeStatus(raw: ExportStatusResponse | null | undefined): { status: UiStatus; download_url: string | null } {
		if (!raw || raw.status === 'no_pending_export' || !raw.status) {
			return { status: 'none', download_url: null };
		}
		if (raw.status === 'completed') return { status: 'ready', download_url: raw.download_url ?? null };
		if (raw.status === 'failed') return { status: 'failed', download_url: null };
		// cancelled → user can re-request, so treat as 'none'
		if (raw.status === 'cancelled') return { status: 'none', download_url: null };
		// pending, processing → pending
		return { status: 'pending', download_url: null };
	}

	const initialConfig = untrack(() => payload?.consentConfig);
	if (initialConfig?.consent_types?.length) consentTypes = initialConfig.consent_types;
	if (initialConfig?.policy_version) policyVersion = initialConfig.policy_version;

	const consentMap = $state<Map<string, boolean>>(untrack(() => buildConsentMap(payload?.consents ?? [])));
	const initialNorm = untrack(() => normalizeStatus(payload?.exportStatus));
	let uiStatus = $state<UiStatus>(initialNorm.status);
	let downloadUrl = $state<string | null>(initialNorm.download_url);

	// Re-seed on SSR payload change (navigation).
	$effect(() => {
		const p = data as DataPrivacyData | null;
		consentTypes = p?.consentConfig?.consent_types ?? [];
		policyVersion = p?.consentConfig?.policy_version ?? '';
		consentMap.clear();
		for (const [k, v] of buildConsentMap(p?.consents ?? [])) consentMap.set(k, v);
		const norm = normalizeStatus(p?.exportStatus);
		uiStatus = norm.status;
		downloadUrl = norm.download_url;
	});

	let toggling = $state<string | null>(null);
	let consentError = $state('');

	let requesting = $state(false);
	let requestError = $state('');
	let requestSuccess = $state('');

	let cancelling = $state(false);
	let cancelError = $state('');

	/** Toggle a consent type on/off via POST /consent or DELETE /consent/{type}. */
	async function toggleConsent(type: string) {
		if (toggling) return;
		toggling = type;
		consentError = '';
		const wasGranted = consentMap.get(type) ?? false;
		// Optimistic update
		consentMap.set(type, !wasGranted);
		try {
			if (wasGranted) {
				await api.delete(`/consent/${type}`);
			} else {
				await api.post('/consent', {
					consent_type: type,
					granted: true,
					policy_version: policyVersion
				});
			}
		} catch (e) {
			// Revert on error
			consentMap.set(type, wasGranted);
			consentError = t('settings.data_privacy.consent_error', 'Failed to update consent. Try again.');
		}
		toggling = null;
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
			uiStatus = 'pending';
		}
		requesting = false;
	}

	/** Cancel a pending data export — DELETE /me/data-export. */
	async function cancelExport() {
		if (cancelling) return;
		cancelling = true;
		cancelError = '';
		const res = await cancelDataExport();
		if (res.error) {
			cancelError = t('settings.data_privacy.cancel_error', 'Failed to cancel export. Try again.');
		} else {
			uiStatus = 'none';
			downloadUrl = null;
			requestSuccess = '';
		}
		cancelling = false;
	}

	// Refresh consent types + policy version client-side (server may have
	// already seeded them). No hardcoded fallback is kept per Rule 16.
	onMount(async () => {
		try {
			const res = await api.get<{ consent_types: string[]; policy_version: string }>('/consent/config');
			if (res.data) {
				if (res.data.consent_types?.length) consentTypes = res.data.consent_types;
				if (res.data.policy_version) policyVersion = res.data.policy_version;
				// Rebuild the map so new types default to granted.
				const next = buildConsentMap(payload?.consents ?? []);
				consentMap.clear();
				for (const [k, v] of next) consentMap.set(k, v);
			}
		} catch {
			// The server-loaded config is already in place; leave it.
		}
	});

	// Poll export status while pending (every 30s).
	onMount(() => {
		if (uiStatus !== 'pending') return;
		const interval = setInterval(async () => {
			if (uiStatus !== 'pending') {
				clearInterval(interval);
				return;
			}
			const res = await api.get<ExportStatusResponse>('/me/data-export/status');
			if (res.data) {
				const norm = normalizeStatus(res.data);
				uiStatus = norm.status;
				downloadUrl = norm.download_url;
				if (norm.status !== 'pending') clearInterval(interval);
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
		{#if consentError}
			<p class="mb-2 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-xs text-red-500" role="alert">{consentError}</p>
		{/if}
		<ul class="space-y-2">
			{#each consentTypes as type (type)}
				{@const granted = consentMap.get(type) ?? false}
				<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2">
					<span class="min-w-0 flex-1">
						<span class="block text-sm font-medium text-[var(--color-text)]">{t(`consent.${type}.title`, type)}</span>
						<span class="block text-xs text-[var(--color-muted)]">{t(`consent.${type}.desc`, '')}</span>
					</span>
					<Switch.Root
						checked={granted}
						onCheckedChange={() => toggleConsent(type)}
						disabled={toggling === type}
						class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors data-[state=checked]:bg-[var(--color-primary)] data-[state=unchecked]:bg-[var(--color-border)]"
					>
						<Switch.Thumb
							class="inline-block h-4 w-4 transform rounded-full bg-white transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0.5"
						/>
					</Switch.Root>
				</li>
			{/each}
		</ul>
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

		{#if uiStatus === 'ready'}
			<a
				href={buildApiUrl('/me/data-export/download', apiBaseUrl)}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
			>
				<Download size={14} /> {t('settings.data_privacy.download_ready', 'Download ready')}
			</a>
		{:else if uiStatus === 'pending'}
			<div class="flex flex-wrap items-center gap-3">
				<p class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
					<Loader2 size={14} class="animate-spin" />
					{t('settings.data_privacy.export_pending', 'Export in progress...')}
				</p>
				<button
					type="button"
					onclick={cancelExport}
					disabled={cancelling}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
				>
					{#if cancelling}
						<Loader2 size={14} class="animate-spin" />
					{:else}
						<X size={14} />
					{/if}
					{t('settings.data_privacy.cancel_export', 'Cancel')}
				</button>
			</div>
			{#if cancelError}
				<p class="mt-2 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-xs text-red-500" role="alert">{cancelError}</p>
			{/if}
		{:else if uiStatus === 'failed'}
			<p class="mb-2 text-sm text-red-500">{t('settings.data_privacy.export_failed', 'Export failed. Please try again.')}</p>
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
