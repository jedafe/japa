<script lang="ts">
	import { untrack, onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { mainAppUrl, buildSeoMeta } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { t, siteName, i18n } from '$lib/i18n';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import type { Embassy, AppointmentAlert, GeoSpaceOption, CatalogOption } from '$lib/types';

	/**
	 * Embassy Appointment Alerter tool — §91, ADR-099.
	 * Destination selector → list of embassies (with booking links) →
	 * create/manage appointment alerts (auth required). Unauthed users see a
	 * registration prompt when they try to create an alert.
	 */
	let { data } = $props();

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let embassies = $state<Embassy[]>(untrack(() => data.embassies));
	let channelOptions = $state<CatalogOption[]>(untrack(() => data.channelOptions));
	let alerts = $state<AppointmentAlert[]>([]);
	let user = $state(untrack(() => data.user));
	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let loadingEmbassies = $state(false);
	let loadingAlerts = $state(false);
	// Request-id guard: increment on each loadEmbassies call; discard stale
	// responses if the user changed destination while a load was in flight.
	let loadEmbassiesReqId = 0;

	// Premium-tier detection (§91, ADR-099): the SMS notification channel is
	// only available to premium users (holding appointment_alert.premium).
	// The backend rejects sms for non-premium users with 403 sms_premium_only.
	const canUseSMS = $derived(!!user?.permissions?.includes('appointment_alert.premium'));

	// Default channels are derived from the API-backed catalog (Rule 16).
	const defaultChannelValues = $derived<string[]>(
		channelOptions.filter((c) => c.default && (c.value !== 'sms' || canUseSMS)).map((c) => c.value)
	);

	// Alert creation form state
	let showRegisterPrompt = $state(false);
	let alertFormEmbassy = $state<Embassy | null>(null);
	let alertDateStart = $state('');
	let alertDateEnd = $state('');
	let alertVisaCategory = $state('');
	let alertChannels = $state<string[]>(untrack(() => defaultChannelValues));
	let alertSaving = $state(false);

	// Alert edit state
	let editingAlertId = $state<string | null>(null);
	let editDateStart = $state('');
	let editDateEnd = $state('');
	let editVisaCategory = $state('');
	let editChannels = $state<string[]>([]);

	const seo = $derived(
		buildSeoMeta({
			title: t('embassy_alerter.seo_title', 'Embassy Appointment Alerter — {site} Tools'),
			description: t(
				'embassy_alerter.seo_description',
				'Subscribe to embassy appointment availability alerts. Get notified when a visa slot opens in your date range.'
			),
			path: '/embassy-alerter',
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-embassy-alerter.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'Service',
				name: 'Embassy Appointment Alerter',
				provider: { '@type': 'Organization', name: siteName() },
				description: 'Embassy appointment availability alerts for visa slots.'
			}
		})
	);

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
		const res = await api.get<Embassy[]>(`/embassies?dest=${selectedDestId}`);
		// Discard stale response if a newer load was started.
		if (reqId !== loadEmbassiesReqId) return;
		loadingEmbassies = false;
		if (!res.error) embassies = res.data ?? [];
		else embassies = [];
	}

	function onDestChange(id: string) {
		selectedDestId = id;
		const url = new URL(page.url);
		if (id) url.searchParams.set('dest', id);
		else url.searchParams.delete('dest');
		replaceState(url, {});
		loadEmbassies();
	}

	function openAlertForm(embassy: Embassy) {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		alertFormEmbassy = embassy;
		alertDateStart = '';
		alertDateEnd = '';
		alertVisaCategory = '';
		alertChannels = [...defaultChannelValues];
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
		if (!alertFormEmbassy || !user) return;
		if (!alertDateStart || !alertDateEnd) return;
		alertSaving = true;
		const res = await api.post<AppointmentAlert>('/me/appointment-alerts', {
			destination_space_id: selectedDestId,
			embassy_slug: alertFormEmbassy.slug,
			visa_category: alertVisaCategory || undefined,
			date_range_start: alertDateStart,
			date_range_end: alertDateEnd,
			notification_channels: alertChannels
		});
		alertSaving = false;
		if (!res.error && res.data) {
			alerts = [...alerts, res.data];
			closeAlertForm();
		}
	}

	async function deleteAlert(id: string) {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		const res = await api.delete(`/me/appointment-alerts/${id}`);
		if (!res.error) alerts = alerts.filter((a) => a.id !== id);
	}

	function startEditAlert(alert: AppointmentAlert) {
		editingAlertId = alert.id;
		editDateStart = alert.date_range_start;
		editDateEnd = alert.date_range_end;
		editVisaCategory = alert.visa_category ?? '';
		editChannels = [...alert.notification_channels];
	}

	async function saveAlertEdit(id: string) {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		const res = await api.patch<AppointmentAlert>(`/me/appointment-alerts/${id}`, {
			date_range_start: editDateStart,
			date_range_end: editDateEnd,
			visa_category: editVisaCategory || undefined,
			notification_channels: editChannels
		});
		if (!res.error && res.data) {
			alerts = alerts.map((a) => (a.id === id ? res.data! : a));
			editingAlertId = null;
		}
	}

	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'active':
				return 'bg-success/10 text-success';
			case 'fired':
				return 'bg-primary/10 text-primary';
			case 'expired':
				return 'bg-surface-subtle text-muted';
			default:
				return 'bg-surface-subtle text-muted';
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
		if (!user) return;
		loadingAlerts = true;
		const res = await api.get<AppointmentAlert[]>('/me/appointment-alerts');
		loadingAlerts = false;
		if (!res.error) alerts = res.data ?? [];
		else alerts = [];
	}

	onMount(() => {
		// Embassies are already populated from SSR when initialDestId was set.
		// User alerts are fetched client-side to avoid SSR user-data leakage.
		loadAlerts();
	});
</script>

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

<div class="mx-auto max-w-5xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
		<div class="mb-8">
			<h1 class="text-2xl sm:text-3xl font-bold text-text">
				{t('embassy_alerter.title', 'Embassy Appointment Alerter')}
			</h1>
			<p class="mt-2 text-text-subtle">
				{t('embassy_alerter.subtitle', 'Get notified when embassy appointment slots open in your date range.')}
			</p>
		</div>

		{#if destinations.length === 0}
			<EmptyState
				icon="bell-ring"
				title={t('embassy_alerter.no_destinations', 'No destinations available')}
				description={t('embassy_alerter.no_destinations_desc', 'Embassy data hasn’t been configured yet. Please check back later.')}
			/>
		{:else}
			<!-- Destination selector (geo-aware pattern — first input) -->
			<div class="mb-6">
				<DestinationSelector {destinations} bind:selectedId={selectedDestId} onchange={onDestChange} />
			</div>

			{#if !selectedDestId}
				<EmptyState
					icon="map-pin"
					title={t('embassy_alerter.select_destination', 'Select a destination country')}
					description={t('embassy_alerter.select_destination_desc', 'Choose a country above to see available embassies.')}
				/>
			{:else}
				<div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
					<!-- Embassy list -->
					<div class="lg:col-span-2">
						<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
							{t('embassy_alerter.embassies', 'Embassies')}
						</h2>
						{#if loadingEmbassies}
							<div class="space-y-3">
								<LoadingSkeleton height="80px" />
								<LoadingSkeleton height="80px" />
							</div>
						{:else if embassies.length === 0}
							<EmptyState
								icon="building"
								title={t('embassy_alerter.no_embassies', 'No embassies available')}
								description={t('embassy_alerter.no_embassies_desc', 'No embassies are being monitored for this destination yet.')}
							/>
						{:else}
							<div class="space-y-3">
								{#each embassies as embassy (embassy.id)}
									<div class="p-4 bg-surface border border-border rounded-lg">
										<div class="flex items-start justify-between gap-3">
											<div class="min-w-0">
												<h3 class="text-sm font-semibold text-text">{embassy.name}</h3>
												<div class="mt-1.5 flex items-center gap-3 text-xs text-muted">
													{#if embassy.booking_url}
														<a
															href={embassy.booking_url}
															target="_blank"
															rel="noopener noreferrer"
															class="inline-flex items-center gap-1 text-[var(--tool-accent)] hover:underline"
														>
															<Icon name="external-link" size={12} />
															{t('embassy_alerter.booking_page', 'Booking page')}
														</a>
													{/if}
													<span class="flex items-center gap-1">
														<Icon name="clock" size={12} />
														{t('embassy_alerter.polls_every', 'Polls every')} {formatPollInterval(embassy.poll_interval_seconds)}
													</span>
													<span class="flex items-center gap-1">
														<Icon name="calendar-clock" size={12} />
														{t('embassy_alerter.last_polled', 'Last checked')}: {formatDateTime(embassy.last_polled_at)}
													</span>
												</div>
											</div>
											<button
												onclick={() => openAlertForm(embassy)}
												class="flex-shrink-0 inline-flex items-center gap-1.5 px-3 py-1.5 bg-[var(--tool-accent)] text-white rounded-lg text-xs font-medium hover:bg-[var(--tool-accent-hover)] transition-colors"
											>
												<Icon name="bell-plus" size={14} />
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
						<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
							{t('embassy_alerter.my_alerts', 'My alerts')}
						</h2>
						{#if !user}
							<div class="p-4 bg-surface border border-border rounded-lg text-center">
								<p class="text-sm text-text-subtle mb-3">
									{t('embassy_alerter.signin_for_alerts', 'Sign in to create appointment alerts and get notified when slots open.')}
								</p>
								<a
									href={mainAppUrl('/login')}
									class="inline-flex items-center gap-1.5 px-4 py-2 bg-primary text-primary-contrast rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors"
								>
									{t('common.sign_in', 'Sign in')}
								</a>
							</div>
						{:else if destAlerts.length === 0}
							<div class="p-4 bg-surface border border-border rounded-lg text-center">
								<p class="text-sm text-text-subtle">
									{t('embassy_alerter.no_alerts', 'No alerts yet. Create one from an embassy above.')}
								</p>
							</div>
						{:else}
							<div class="space-y-3">
								{#each destAlerts as alert (alert.id)}
									<div class="p-3 bg-surface border border-border rounded-lg">
										{#if editingAlertId === alert.id}
											<!-- Edit mode (PATCH /me/appointment-alerts/:id — §91) -->
											<div class="space-y-2">
												<div class="flex gap-2">
													<input
														type="date"
														bind:value={editDateStart}
														class="flex-1 px-2 py-1 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
													/>
													<input
														type="date"
														bind:value={editDateEnd}
														class="flex-1 px-2 py-1 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
													/>
												</div>
												<input
													type="text"
													bind:value={editVisaCategory}
													placeholder={t('embassy_alerter.visa_category_optional', 'Visa category (optional)')}
													class="w-full px-2 py-1 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
												/>
												<div class="flex gap-3 text-xs">
													{#each channelOptions as opt (opt.value)}
														{#if opt.value !== 'sms' || canUseSMS}
															<label class="flex items-center gap-1 text-text">
																<input
																	type="checkbox"
																	checked={editChannels.includes(opt.value)}
																	onchange={() => (editChannels = toggleChannel(editChannels, opt.value))}
																	class="accent-[var(--tool-accent)]"
																/>
																{t(`embassy_alerter.${opt.value}`, opt.label)}
															</label>
														{/if}
													{/each}
												</div>
												<div class="flex gap-2">
													<button
														onclick={() => saveAlertEdit(alert.id)}
														class="px-3 py-1 text-sm bg-[var(--tool-accent)] text-white rounded hover:bg-[var(--tool-accent-hover)]"
													>
														{t('common.save', 'Save')}
													</button>
													<button
														onclick={() => (editingAlertId = null)}
														class="px-3 py-1 text-sm text-muted hover:text-text"
													>
														{t('common.cancel', 'Cancel')}
													</button>
												</div>
											</div>
										{:else}
											<div class="flex items-start justify-between gap-2">
												<div class="text-sm min-w-0">
													<div class="flex items-center gap-2">
														<span class="font-medium text-text">{alert.embassy_slug}</span>
														<span class="text-xs px-2 py-0.5 rounded-full {statusBadgeClass(alert.status)}">
															{alert.status}
														</span>
													</div>
													<p class="text-xs text-muted mt-1">
														{alert.date_range_start} → {alert.date_range_end}
													</p>
													{#if alert.visa_category}
														<p class="text-xs text-muted">{alert.visa_category}</p>
													{/if}
													{#if alert.fired_at}
														<p class="text-xs text-primary mt-1">
															{t('embassy_alerter.fired_at', 'Fired')}: {formatDateTime(alert.fired_at)}
														</p>
													{/if}
													<div class="flex gap-1.5 mt-1.5">
														{#each alert.notification_channels as ch (ch)}
															<span class="text-xs px-1.5 py-0.5 bg-surface-subtle text-muted rounded">{ch}</span>
														{/each}
													</div>
												</div>
												<div class="flex items-center gap-1 flex-shrink-0">
													<button
														onclick={() => startEditAlert(alert)}
														class="p-1.5 text-muted hover:text-text transition-colors"
														aria-label={t('embassy_alerter.edit_alert', 'Edit alert')}
													>
														<Icon name="pencil" size={14} />
													</button>
													<button
														onclick={() => deleteAlert(alert.id)}
														class="p-1.5 text-muted hover:text-danger transition-colors"
														aria-label={t('embassy_alerter.delete_alert', 'Delete alert')}
													>
														<Icon name="trash-2" size={14} />
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
					class="bg-surface rounded-lg p-6 max-w-md w-full"
					onclick={(e) => e.stopPropagation()}
					onkeydown={(e) => e.stopPropagation()}
					role="dialog"
					aria-modal="true"
					tabindex="-1"
				>
					<div class="flex items-center justify-between mb-4">
						<h3 class="text-lg font-semibold text-text">
							{t('embassy_alerter.create_alert_for', 'Create alert for')}
						</h3>
						<button
							onclick={closeAlertForm}
							class="text-muted hover:text-text"
							aria-label={t('common.close', 'Close')}
						>
							<Icon name="x" size={20} />
						</button>
					</div>
					<p class="text-sm text-text-subtle mb-4">{alertFormEmbassy.name}</p>
					<div class="space-y-4">
						<div>
							<label class="block text-sm font-medium text-text mb-1" for="alert-date-start">
								{t('embassy_alerter.date_range_start', 'Date range start')}
							</label>
							<input
								id="alert-date-start"
								type="date"
								bind:value={alertDateStart}
								class="w-full px-3 py-2 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
							/>
						</div>
						<div>
							<label class="block text-sm font-medium text-text mb-1" for="alert-date-end">
								{t('embassy_alerter.date_range_end', 'Date range end')}
							</label>
							<input
								id="alert-date-end"
								type="date"
								bind:value={alertDateEnd}
								class="w-full px-3 py-2 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
							/>
						</div>
						<div>
							<label class="block text-sm font-medium text-text mb-1" for="alert-visa-cat">
								{t('embassy_alerter.visa_category_optional', 'Visa category (optional)')}
							</label>
							<input
								id="alert-visa-cat"
								type="text"
								bind:value={alertVisaCategory}
								placeholder={t('embassy_alerter.visa_category_placeholder', 'e.g. Tourist, Student')}
								class="w-full px-3 py-2 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
							/>
						</div>
						<div>
							<span class="block text-sm font-medium text-text mb-1">
								{t('embassy_alerter.notifications', 'Notifications')}
							</span>
							<div class="flex gap-4">
								{#each channelOptions as opt (opt.value)}
									{#if opt.value !== 'sms' || canUseSMS}
										<label class="flex items-center gap-2 text-sm text-text">
											<input
												type="checkbox"
												checked={alertChannels.includes(opt.value)}
												onchange={() => (alertChannels = toggleChannel(alertChannels, opt.value))}
												class="accent-[var(--tool-accent)]"
											/>
											{t(`embassy_alerter.${opt.value}`, opt.label)}
											{#if opt.value === 'sms'}
												<span class="text-xs text-muted">({t('embassy_alerter.premium', 'premium')})</span>
											{/if}
										</label>
									{/if}
								{/each}
							</div>
						</div>
						<button
							onclick={createAlert}
							disabled={!alertDateStart || !alertDateEnd || alertSaving}
							class="w-full px-4 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
						>
							{alertSaving ? t('common.saving', 'Saving…') : t('embassy_alerter.create_alert', 'Create alert')}
						</button>
					</div>
				</div>
			</div>
		{/if}

		{#if showRegisterPrompt}
			<RegistrationPrompt
				open={showRegisterPrompt}
				onClose={() => (showRegisterPrompt = false)}
				icon="bell-plus"
				title={t('register.create_alerts', 'Create appointment alerts')}
			description={t('register.create_alerts_desc', 'Create a free account to set embassy appointment alerts and get notified when slots open.')}
		/>
	{/if}
	{/if}
</div>