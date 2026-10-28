<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { untrack, onMount } from 'svelte';
	import { t, siteName, i18n } from '$lib/i18n';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import {
		BellRing,
		MapPin,
		Building2,
		ExternalLink,
		Clock,
		CalendarClock,
		BellPlus,
		Pencil,
		Trash2,
		X,
		Lock
	} from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { Embassy, AppointmentAlert, GeoCountry, CatalogOption } from '$lib/types';
	import type { PageData } from './$types';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';

	/**
	 * Embassy Appointment Alerter tool — §91, ADR-099.
	 * Destination selector → list of embassies (with booking links) →
	 * create/manage appointment alerts (auth required). Unauthed users see a
	 * registration prompt when they try to create an alert.
	 */
	let { data }: { data: PageData } = $props();

	const title = $derived(t('embassy_alerter.title', 'Embassy Appointment Alerter'));
	const description = $derived(
		t(
			'embassy_alerter.subtitle',
			'Get notified when embassy appointment slots open in your date range.'
		)
	);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

	let destinations = $state<GeoCountry[]>(untrack(() => data.destinations));
	let embassies = $state<Embassy[]>(untrack(() => data.embassies));
	let alerts = $state<AppointmentAlert[]>([]);
	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let loadingEmbassies = $state(false);
	let loadingAlerts = $state(false);
	let error = $state<string | null>(untrack(() => data.loadError ?? null));
	const channelOptions = $derived<CatalogOption[]>(data.channelOptions ?? []);
	const defaultChannels = $derived<string[]>(
		channelOptions.filter((c) => c.default).map((c) => c.value)
	);
	// Request-id guard: increment on each loadEmbassies call; discard stale
	// responses if the user changed destination while a load was in flight.
	let loadEmbassiesReqId = 0;

	// Alert creation form state
	let showRegisterPrompt = $state(false);
	let alertFormEmbassy = $state<Embassy | null>(null);
	let alertDateStart = $state('');
	let alertDateEnd = $state('');
	let alertVisaCategory = $state('');
	let alertChannels = $state<string[]>(untrack(() => defaultChannels));
	let alertSaving = $state(false);

	// Alert edit state
	let editingAlertId = $state<string | null>(null);
	let editDateStart = $state('');
	let editDateEnd = $state('');
	let editVisaCategory = $state('');
	let editChannels = $state<string[]>([]);

	// Premium-tier detection (§91, ADR-099): the SMS notification channel is
	// only available to premium users (holding appointment_alert.premium).
	// The backend rejects sms for non-premium users with 403 sms_premium_only.
	const canUseSMS = $derived(data.userPermissions?.includes('appointment_alert.premium') ?? false);

	// Filter alerts to the current destination for display
	const destAlerts = $derived(
		selectedDestId ? alerts.filter((a) => a.destination_space_id === selectedDestId) : []
	);

	async function loadEmbassies() {
		if (!selectedDestId) {
			embassies = [];
			return;
		}
		const reqId = ++loadEmbassiesReqId;
		loadingEmbassies = true;
		error = null;
		const res = await api.get<Embassy[]>(`/embassies?dest=${selectedDestId}`);
		// Discard stale response if a newer load was started.
		if (reqId !== loadEmbassiesReqId) return;
		loadingEmbassies = false;
		if (res.error) {
			error = res.error.message;
			embassies = [];
		} else {
			embassies = res.data ?? [];
		}
	}

	function onDestChange(id: string) {
		selectedDestId = id;
		error = null;
		const url = new URL(page.url);
		if (id) url.searchParams.set('dest', id);
		else url.searchParams.delete('dest');
		replaceState(url, {});
		loadEmbassies();
	}

	function openAlertForm(embassy: Embassy) {
		if (!authed) {
			showRegisterPrompt = true;
			return;
		}
		alertFormEmbassy = embassy;
		alertDateStart = '';
		alertDateEnd = '';
		alertVisaCategory = '';
		alertChannels = defaultChannels;
	}

	function closeAlertForm() {
		alertFormEmbassy = null;
	}

	function toggleChannel(channels: string[], channel: string): string[] {
		return channels.includes(channel)
			? channels.filter((c) => c !== channel)
			: [...channels, channel];
	}

	async function createAlert() {
		if (!alertFormEmbassy || !authed) return;
		if (!alertDateStart || !alertDateEnd) return;
		alertSaving = true;
		error = null;
		const res = await api.post<AppointmentAlert>('/me/appointment-alerts', {
			destination_space_id: selectedDestId,
			embassy_slug: alertFormEmbassy.slug,
			visa_category: alertVisaCategory || undefined,
			date_range_start: alertDateStart,
			date_range_end: alertDateEnd,
			notification_channels: alertChannels
		});
		alertSaving = false;
		if (res.error) {
			error = res.error.message;
		} else if (res.data) {
			alerts = [...alerts, res.data!];
			closeAlertForm();
		}
	}

	async function deleteAlert(id: string) {
		error = null;
		const res = await api.delete(`/me/appointment-alerts/${id}`);
		if (res.error) {
			error = res.error.message;
		} else {
			alerts = alerts.filter((a) => a.id !== id);
		}
	}

	function startEditAlert(alert: AppointmentAlert) {
		editingAlertId = alert.id;
		editDateStart = alert.date_range_start;
		editDateEnd = alert.date_range_end;
		editVisaCategory = alert.visa_category ?? '';
		editChannels = [...alert.notification_channels];
	}

	async function saveAlertEdit(id: string) {
		error = null;
		const res = await api.patch<AppointmentAlert>(`/me/appointment-alerts/${id}`, {
			date_range_start: editDateStart,
			date_range_end: editDateEnd,
			visa_category: editVisaCategory || undefined,
			notification_channels: editChannels
		});
		if (res.error) {
			error = res.error.message;
		} else if (res.data) {
			const updated = res.data!;
			alerts = alerts.map((a) => (a.id === id ? updated : a));
			editingAlertId = null;
		}
	}

	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'active':
				return 'bg-[var(--color-success)]/10 text-[var(--color-success)]';
			case 'fired':
				return 'bg-[var(--color-info)]/10 text-[var(--color-info)]';
			case 'expired':
				return 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]';
			default:
				return 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]';
		}
	}

	function formatPollInterval(seconds: number): string {
		if (seconds < 60) return `${seconds}s`;
		const mins = Math.round(seconds / 60);
		if (mins < 60) return `${mins}m`;
		const hours = Math.round(mins / 60);
		return `${hours}h`;
	}

	function formatDateTime(iso: string | null | undefined): string {
		if (!iso) return '—';
		return new Date(iso).toLocaleString(i18n.locale, {
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	async function loadAlerts() {
		if (!authed) return;
		loadingAlerts = true;
		error = null;
		const res = await api.get<AppointmentAlert[]>('/me/appointment-alerts');
		loadingAlerts = false;
		if (res.error) {
			error = res.error.message;
			alerts = [];
		} else {
			alerts = res.data ?? [];
		}
	}

	onMount(() => {
		// Embassies are already populated from SSR when initialDestId was set.
		// User alerts are fetched client-side to avoid SSR user-data leakage.
		loadAlerts();
	});

	// Autocomplete fetchers — filter the already-loaded destinations client-side.
	function fetchDestinations(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? destinations
			: destinations.filter((d) => d.name.toLowerCase().includes(query) || d.geo_code.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((d) => ({ id: d.id, label: d.name, sublabel: d.geo_code })));
	}
	function fetchDestinationsInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinations('');
	}
	const selectedDestLabel = $derived(
		selectedDestId ? destinations.find((d) => d.id === selectedDestId)?.name ?? null : null
	);
</script>

<svelte:head>
	<title>{title} — {siteName()} Tools</title>
	<meta
		name="description"
		content={t(
			'embassy_alerter.seo_description',
			'Subscribe to embassy appointment availability alerts. Get notified when a visa slot opens in your date range.'
		)}
	/>
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content="{title} — {siteName()} Tools" />
	<meta
		property="og:description"
		content={t(
			'embassy_alerter.seo_description',
			'Subscribe to embassy appointment availability alerts. Get notified when a visa slot opens in your date range.'
		)}
	/>
	<meta property="og:url" content={page.url.href} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content="{title} — {siteName()} Tools" />
	<meta
		name="twitter:description"
		content={t(
			'embassy_alerter.seo_description',
			'Subscribe to embassy appointment availability alerts. Get notified when a visa slot opens in your date range.'
		)}
	/>
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<BellRing size={20} /> {title}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else if destinations.length === 0}
		<div class="p-4">
			<EmptyState
				icon={BellRing}
				title={t('embassy_alerter.no_destinations', 'No destinations available')}
				body={t(
					'embassy_alerter.no_destinations_desc',
					'Embassy data hasn’t been configured yet. Please check back later.'
				)}
			/>
		</div>
	{:else}
		<!-- Destination selector (geo-aware pattern — first input) -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<label
				for="ea-dest"
				class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]"
			>
				{t('geo.destination', 'Destination:')}
			
				<HintIcon text={t('tools.embassy.dest_hint', 'The country whose embassy alerts you want.')}  />
</label>
			<div class="mt-2">
				<Autocomplete
					placeholder={t('geo.select_destination', 'Select a country…')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinations}
					initialFetcher={fetchDestinationsInitial}
					selectedId={selectedDestId || null}
					selectedLabel={selectedDestLabel}
					onSelect={(item) => onDestChange(item.id)}
					onClear={() => onDestChange('')}
				/>
			</div>
		</div>

		{#if error}
			<div class="p-4 pb-0">
				<div class="rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
					{error}
				</div>
			</div>
		{/if}

		{#if !selectedDestId}
			<div class="p-4">
				<EmptyState
					icon={MapPin}
					title={t('embassy_alerter.select_destination', 'Select a destination country')}
					body={t(
						'embassy_alerter.select_destination_desc',
						'Choose a country above to see available embassies.'
					)}
				/>
			</div>
		{:else}
			<div class="grid grid-cols-1 lg:grid-cols-3 gap-6 p-4">
				<!-- Embassy list -->
				<div class="lg:col-span-2">
					<h2
						class="text-xs font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-3"
					>
						{t('embassy_alerter.embassies', 'Embassies')}
					</h2>
					{#if loadingEmbassies}
						<LoadingSkeleton variant="card" count={2} />
					{:else if embassies.length === 0}
						<EmptyState
							icon={Building2}
							title={t('embassy_alerter.no_embassies', 'No embassies available')}
							body={t(
								'embassy_alerter.no_embassies_desc',
								'No embassies are being monitored for this destination yet.'
							)}
						/>
					{:else}
						<div class="space-y-3">
							{#each embassies as embassy (embassy.id)}
								<div
									class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
								>
									<div class="flex items-start justify-between gap-3">
										<div class="min-w-0">
											<h3 class="text-sm font-semibold text-[var(--color-text)]">
												{embassy.name}
											</h3>
											<div
												class="mt-1.5 flex items-center gap-3 text-xs text-[var(--color-muted)]"
											>
												{#if embassy.booking_url}
													<a
														href={embassy.booking_url}
														target="_blank"
														rel="noopener noreferrer"
														class="inline-flex items-center gap-1 text-[var(--color-primary)] hover:underline"
													>
														<ExternalLink class="rtl:-scale-x-100" size={12} />
														{t('embassy_alerter.booking_page', 'Booking page')}
													</a>
												{/if}
												<span class="flex items-center gap-1">
													<Clock size={12} />
													{t('embassy_alerter.polls_every', 'Polls every')}
													{formatPollInterval(embassy.poll_interval_seconds)}
												</span>
												<span class="flex items-center gap-1">
													<CalendarClock size={12} />
													{t('embassy_alerter.last_polled', 'Last checked')}:
													{formatDateTime(embassy.last_polled_at)}
												</span>
											</div>
										</div>
										<button
											type="button"
											onclick={() => openAlertForm(embassy)}
											class="flex-shrink-0 inline-flex items-center gap-1.5 px-3 py-1.5 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-xs font-medium hover:bg-[var(--color-primary-hover)] transition-colors"
										>
											<BellPlus size={14} />
											{t('embassy_alerter.create_alert', 'Create alert')}
										</button>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</div>

				<!-- Alerts sidebar -->
				<div>
					<h2
						class="text-xs font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-3"
					>
						{t('embassy_alerter.my_alerts', 'My alerts')}
					</h2>
					{#if !authed}
						<div
							class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-center"
						>
							<p class="text-sm text-[var(--color-muted)] mb-3">
								{t(
									'embassy_alerter.signin_for_alerts',
									'Sign in to create appointment alerts and get notified when slots open.'
								)}
							</p>
							<a
								href="/login"
								class="inline-flex items-center gap-1.5 px-4 py-2 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-primary-hover)] transition-colors"
							>
								{t('common.sign_in', 'Sign in')}
							</a>
						</div>
					{:else if loadingAlerts}
						<LoadingSkeleton variant="lines" count={2} />
					{:else if destAlerts.length === 0}
						<div
							class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-center"
						>
							<p class="text-sm text-[var(--color-muted)]">
								{t('embassy_alerter.no_alerts', 'No alerts yet. Create one from an embassy above.')}
							</p>
						</div>
					{:else}
						<div class="space-y-3">
							{#each destAlerts as alert (alert.id)}
								<div
									class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
								>
									{#if editingAlertId === alert.id}
										<!-- Edit mode (PATCH /me/appointment-alerts/:id — §91) -->
										<div class="space-y-2">
											<div class="flex gap-2">
												<input
													type="date"
													bind:value={editDateStart}
													class="flex-1 px-2 py-1 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] focus:border-[var(--color-primary)] focus:outline-none"
												/>
												<input
													type="date"
													bind:value={editDateEnd}
													class="flex-1 px-2 py-1 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] focus:border-[var(--color-primary)] focus:outline-none"
												/>
											</div>
											<input
												type="text"
												bind:value={editVisaCategory}
												placeholder={t(
													'embassy_alerter.visa_category_optional',
													'Visa category (optional)'
												)}
												class="w-full px-2 py-1 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] focus:border-[var(--color-primary)] focus:outline-none"
											/>
											<div class="flex gap-3 text-xs">
												{#each channelOptions as opt (opt.value)}
													{#if opt.value !== 'sms' || canUseSMS}
														<label class="flex items-center gap-1 text-[var(--color-text)]">
															<input
																type="checkbox"
																checked={editChannels.includes(opt.value)}
																onchange={() => (editChannels = toggleChannel(editChannels, opt.value))}
																class="accent-[var(--color-primary)]"
															/>
															{opt.label}
														</label>
													{/if}
												{/each}
											</div>
											<div class="flex gap-2">
												<button
													type="button"
													onclick={() => saveAlertEdit(alert.id)}
													class="px-3 py-1 text-sm bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] hover:bg-[var(--color-primary-hover)]"
												>
													{t('common.save', 'Save')}
												</button>
												<button
													type="button"
													onclick={() => (editingAlertId = null)}
													class="px-3 py-1 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
												>
													{t('common.cancel', 'Cancel')}
												</button>
											</div>
										</div>
									{:else}
										<div class="flex items-start justify-between gap-2">
											<div class="text-sm min-w-0">
												<div class="flex items-center gap-2">
													<span class="font-medium text-[var(--color-text)]"
														>{alert.embassy_slug}</span
													>
													<span
														class="text-xs px-2 py-0.5 rounded-full {statusBadgeClass(alert.status)}"
													>
														{alert.status}
													</span>
												</div>
												<p class="text-xs text-[var(--color-muted)] mt-1">
													{alert.date_range_start} → {alert.date_range_end}
												</p>
												{#if alert.visa_category}
													<p class="text-xs text-[var(--color-muted)]">{alert.visa_category}</p>
												{/if}
												{#if alert.fired_at}
													<p class="text-xs text-[var(--color-info)] mt-1">
														{t('embassy_alerter.fired_at', 'Fired')}:
														{formatDateTime(alert.fired_at)}
													</p>
												{/if}
												<div class="flex gap-1.5 mt-1.5">
													{#each alert.notification_channels as ch (ch)}
														<span
															class="text-xs px-1.5 py-0.5 bg-[var(--color-surface-subtle)] text-[var(--color-muted)] rounded"
															>{ch}</span
														>
													{/each}
												</div>
											</div>
											<div class="flex items-center gap-1 flex-shrink-0">
												<button
													type="button"
													onclick={() => startEditAlert(alert)}
													class="p-1.5 text-[var(--color-muted)] hover:text-[var(--color-text)] transition-colors"
													aria-label={t('embassy_alerter.edit_alert', 'Edit alert')}
												>
													<Pencil size={14} />
												</button>
												<button
													type="button"
													onclick={() => deleteAlert(alert.id)}
													class="p-1.5 text-[var(--color-muted)] hover:text-[var(--color-danger)] transition-colors"
													aria-label={t('embassy_alerter.delete_alert', 'Delete alert')}
												>
													<Trash2 size={14} />
												</button>
											</div>
										</div>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				</div>
			</div>
		{/if}
	{/if}

	<!-- Alert creation form modal -->
	{#if alertFormEmbassy}
		<div
			class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
			onclick={closeAlertForm}
			onkeydown={(e) => e.key === 'Escape' && closeAlertForm()}
			role="presentation"
		>
			<div
				class="bg-[var(--color-surface)] rounded-[var(--radius-card)] p-6 max-w-md w-full"
				onclick={(e) => e.stopPropagation()}
				onkeydown={(e) => e.stopPropagation()}
				role="dialog"
				aria-modal="true"
				tabindex="-1"
			>
				<div class="flex items-center justify-between mb-4">
					<h3 class="text-lg font-semibold text-[var(--color-text)]">
						{t('embassy_alerter.create_alert_for', 'Create alert for')}
					</h3>
					<button
						type="button"
						onclick={closeAlertForm}
						class="text-[var(--color-muted)] hover:text-[var(--color-text)]"
						aria-label={t('common.close', 'Close')}
					>
						<X size={20} />
					</button>
				</div>
				<p class="text-sm text-[var(--color-muted)] mb-4">{alertFormEmbassy.name}</p>
				<div class="space-y-4">
					<div>
						<label
							class="block text-sm font-medium text-[var(--color-text)] mb-1"
							for="alert-date-start"
						>
							{t('embassy_alerter.date_range_start', 'Date range start')}
						</label>
						<input
							id="alert-date-start"
							type="date"
							bind:value={alertDateStart}
							class="w-full px-3 py-2 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
						/>
					</div>
					<div>
						<label
							class="block text-sm font-medium text-[var(--color-text)] mb-1"
							for="alert-date-end"
						>
							{t('embassy_alerter.date_range_end', 'Date range end')}
						</label>
						<input
							id="alert-date-end"
							type="date"
							bind:value={alertDateEnd}
							class="w-full px-3 py-2 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
						/>
					</div>
					<div>
						<label
							class="block text-sm font-medium text-[var(--color-text)] mb-1"
							for="alert-visa-cat"
						>
							{t('embassy_alerter.visa_category_optional', 'Visa category (optional)')}
						</label>
						<input
							id="alert-visa-cat"
							type="text"
							bind:value={alertVisaCategory}
							placeholder={t('embassy_alerter.visa_category_placeholder', 'e.g. Tourist, Student')}
							class="w-full px-3 py-2 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
						/>
					</div>
					<div>
						<span class="block text-sm font-medium text-[var(--color-text)] mb-1">
							{t('embassy_alerter.notifications', 'Notifications')}
						</span>
						<div class="flex gap-4">
							{#each channelOptions as opt (opt.value)}
								{#if opt.value !== 'sms' || canUseSMS}
									<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
										<input
											type="checkbox"
											checked={alertChannels.includes(opt.value)}
											onchange={() => (alertChannels = toggleChannel(alertChannels, opt.value))}
											class="accent-[var(--color-primary)]"
										/>
										{opt.label}
										{#if opt.value === 'sms'}
											<span class="text-xs text-[var(--color-muted)]">({t('embassy_alerter.premium', 'premium')})</span>
										{/if}
									</label>
								{/if}
							{/each}
						</div>
					</div>
					<button
						type="button"
						onclick={createAlert}
						disabled={!alertDateStart || !alertDateEnd || alertSaving}
						class="w-full px-4 py-2 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-primary-hover)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
					>
						{alertSaving
							? t('common.saving', 'Saving…')
							: t('embassy_alerter.create_alert', 'Create alert')}
					</button>
				</div>
			</div>
		</div>
	{/if}

	{#if showRegisterPrompt}
		<div class="p-4">
			<div
				class="mb-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-muted)]"
			>
				{t('register.create_alerts_desc', 'Create a free account to set embassy appointment alerts and get notified when slots open.')}
				<a href="/login" class="ms-1 text-[var(--color-primary)] hover:underline"
					>{t('common.sign_in', 'Sign in')}</a
				>.
			</div>
		</div>
	{/if}
</div>
