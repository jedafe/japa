<!-- §122 Next-Action rule editor -->
<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { getRule, createRule, updateRule, deleteRule, getVocab, type NextActionRule } from '$lib/api/nextactions';
	import PredicateGroupEditor from '$lib/components/PredicateGroupEditor.svelte';
	import AudienceEditor from '$lib/components/AudienceEditor.svelte';
	import FrequencyEditor from '$lib/components/FrequencyEditor.svelte';

	const id = $derived(page.params.id ?? '');
	const isNew = $derived(id === 'new');

	const DEFAULT_FREQUENCY = { mode: 'recurring', max_impressions: 5, cooldown_hours: 72, snooze_default_hours: 168 };

	let slug = $state('');
	let name = $state('');
	let description = $state('');
	let enabled = $state(false);
	let priority = $state(100);
	let title = $state('');
	let body = $state('');
	let ctaLabel = $state('');
	let ctaUrl = $state('');
	let badge = $state('');
	let channelsSel = $state<string[]>([]);
	let upsell = $state(false);
	let icon = $state('');
	let validFrom = $state('');
	let validUntil = $state('');
	let audienceObj = $state<Record<string, any>>({ user_class: ['regular'] });
	let conditionsObj = $state<Record<string, any>>({ all: [] });
	let completionObj = $state<Record<string, any> | null>(null);
	let completionEnabled = $state(false);
	let frequencyObj = $state<Record<string, any>>({ ...DEFAULT_FREQUENCY });

	let loading = $state(false);
	let pageLoading = $state(true);
	let loadError = $state('');
	let message = $state('');
	let messageType = $state<'success' | 'error' | 'info'>('info');
	let deleteConfirmOpen = $state(false);

	// Fallback mirrors the backend channelSet; /admin/next-actions/vocab is
	// the source of truth (Rule 16) so a new channel can't drift the UI.
	let channelsVocab = $state<string[]>(['popup', 'announcement_bar', 'sidebar', 'in_article', 'notification', 'push', 'email']);
	$effect(() => {
		getVocab().then((r) => {
			if (r.ok && r.data?.channels?.length) channelsVocab = r.data.channels;
		});
	});

	function resetForm() {
		slug = ''; name = ''; description = ''; enabled = false; priority = 100;
		title = ''; body = ''; ctaLabel = ''; ctaUrl = ''; badge = ''; channelsSel = [];
		upsell = false; icon = ''; validFrom = ''; validUntil = '';
		audienceObj = { user_class: ['regular'] };
		conditionsObj = { all: [] };
		completionObj = null;
		completionEnabled = false;
		frequencyObj = { ...DEFAULT_FREQUENCY };
	}

	let fetchSeq = 0;
	$effect(() => {
		// Reset on every id change — without this, navigating rule→rule or
		// rule→new leaves the previous rule's values in the form. seq guards
		// the out-of-order resolution race: a slow getRule for the previous
		// id must not overwrite the freshly loaded rule's fields.
		const seq = ++fetchSeq;
		resetForm();
		loadError = '';
		message = '';
		if (isNew) { pageLoading = false; return; }
		pageLoading = true;
		getRule(id).then((r) => {
			if (seq !== fetchSeq) return;
			pageLoading = false;
			if (!r.ok || !r.data) { loadError = r.ok ? t('nextactions.not_found', 'Rule not found') : (r.error ?? t('nextactions.not_found', 'Rule not found')); return; }
			const d = r.data as NextActionRule;
			slug = d.slug; name = d.name; description = d.description;
			enabled = d.enabled; priority = d.priority;
			upsell = d.upsell ?? false;
			title = d.payload.title; body = d.payload.body;
			ctaLabel = d.payload.cta_label; ctaUrl = d.payload.cta_url;
			badge = d.payload.badge ?? '';
			icon = d.payload.icon ?? '';
			validFrom = toLocalInput(d.valid_from);
			validUntil = toLocalInput(d.valid_until);
			channelsSel = d.channels ?? [];
			audienceObj = (d.audience && typeof d.audience === 'object' ? d.audience : {}) as Record<string, any>;
			conditionsObj = (d.conditions && typeof d.conditions === 'object' ? d.conditions : { all: [] }) as Record<string, any>;
			completionObj = d.completion_signal && typeof d.completion_signal === 'object' ? d.completion_signal : null;
			completionEnabled = !!d.completion_signal;
			frequencyObj = (d.frequency && typeof d.frequency === 'object' ? d.frequency : {}) as Record<string, any>;
		});
	});

	// datetime-local inputs need local "YYYY-MM-DDTHH:mm"; the API wants
	// an RFC3339 timestamp. Empty string clears the field (sends null).
	function toLocalInput(ts: string | null | undefined) {
		if (!ts) return '';
		const d = new Date(ts);
		if (isNaN(d.getTime())) return '';
		const pad = (n: number) => String(n).padStart(2, '0');
		return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
	}
	function toApiTs(local: string) {
		if (!local) return null;
		const d = new Date(local);
		return isNaN(d.getTime()) ? null : d.toISOString();
	}

	async function save() {
		message = '';
		if (channelsSel.length === 0) {
			message = t('nextactions.channels_required', 'Select at least one channel');
			messageType = 'error';
			return;
		}
		try {
			const payload = {
				slug, name, description, enabled, priority, upsell,
				audience: audienceObj,
				conditions: conditionsObj,
				payload: { title, body, cta_label: ctaLabel, cta_url: ctaUrl, icon, ...(badge.trim() ? { badge: badge.trim() } : {}) },
				channels: channelsSel,
				completion_signal: completionEnabled ? completionObj : null,
				frequency: frequencyObj,
				valid_from: toApiTs(validFrom),
				valid_until: toApiTs(validUntil)
			};
			loading = true;
			const r = isNew ? await createRule(payload) : await updateRule(id, payload);
			loading = false;
			if (!r.ok) { message = r.error || t('nextactions.save_failed', 'Save failed'); messageType = 'error'; return; }
			message = t('nextactions.saved', 'Saved'); messageType = 'success';
			if (isNew) goto('/next-actions');
		} catch (e) {
			loading = false;
			message = e instanceof Error ? e.message : t('nextactions.save_failed', 'Save failed');
			messageType = 'error';
		}
	}

	async function remove() {
		const r = await deleteRule(id);
		if (r.ok) goto('/next-actions');
		else { message = r.error || t('nextactions.delete_failed', 'Delete failed'); messageType = 'error'; }
	}
</script>

<PageHeader
	title={isNew ? t('nextactions.new_title', 'New next-action rule') : t('nextactions.edit_title', 'Edit rule — {slug}').replace('{slug}', slug)}
	description={t('nextactions.edit_description', '§122 suggestion rule (JSON fields validated server-side)')}
/>

{#if !isNew && !pageLoading && !loadError}
	<div class="mb-3">
		<button onclick={() => (deleteConfirmOpen = true)} class="border-destructive text-destructive rounded-[var(--radius)] border px-3 py-1.5 text-sm">{t('nextactions.delete_rule', 'Delete rule')}</button>
	</div>
{/if}

{#if message}<AlertBanner variant={messageType} {message} />{/if}

{#if pageLoading}
	<p class="text-muted-foreground mt-4 text-sm">{t('common.loading', 'Loading...')}</p>
{:else if loadError}
	<AlertBanner variant="error" message={loadError} />
{:else}
<form onsubmit={(e) => { e.preventDefault(); void save(); }} class="mt-4 max-w-3xl space-y-4">
	<div class="grid grid-cols-2 gap-4">
		<label class="block text-sm">{t('nextactions.f_slug', 'Slug')}
			<input bind:value={slug} required disabled={!isNew} class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5 font-mono text-xs" />
		</label>
		<label class="block text-sm">{t('nextactions.f_name', 'Name')}
			<input bind:value={name} required class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" />
		</label>
		<label class="block text-sm">{t('nextactions.f_priority', 'Priority (lower = first)')}
			<input bind:value={priority} type="number" class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" />
		</label>
		<label class="mt-6 flex items-center gap-2 text-sm">
			<input bind:checked={enabled} type="checkbox" /> {t('nextactions.f_enabled', 'Enabled')}
		</label>
		<label class="mt-6 flex items-center gap-2 text-sm" title={t('nextactions.upsell_cap_hint', 'Counts toward nextactions.max_upsell_per_day')}>
			<input bind:checked={upsell} type="checkbox" /> {t('nextactions.f_upsell', 'Upsell')}
		</label>
	</div>
	<div class="grid grid-cols-2 gap-4">
		<label class="block text-sm">{t('nextactions.f_valid_from', 'Valid from (optional)')}
			<input bind:value={validFrom} type="datetime-local" class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" />
		</label>
		<label class="block text-sm">{t('nextactions.f_valid_until', 'Valid until (optional)')}
			<input bind:value={validUntil} type="datetime-local" class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" />
		</label>
	</div>
	<label class="block text-sm">{t('nextactions.f_description', 'Description')}
		<input bind:value={description} class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" />
	</label>

	<fieldset class="border-border rounded-[var(--radius-card)] border p-3">
		<legend class="text-muted-foreground px-1 text-xs">{t('nextactions.payload', 'Payload')}</legend>
		<div class="grid grid-cols-2 gap-3">
			<label class="block text-sm">{t('nextactions.f_title', 'Title')} <input bind:value={title} required class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" /></label>
			<label class="block text-sm">{t('nextactions.f_cta_label', 'CTA label')} <input bind:value={ctaLabel} class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" /></label>
			<label class="block text-sm">{t('nextactions.f_cta_url', 'CTA URL')} <input bind:value={ctaUrl} required placeholder="/pricing" class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5 font-mono text-xs" /></label>
			<label class="block text-sm">{t('nextactions.f_body', 'Body')} <input bind:value={body} class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" /></label>
			<label class="block text-sm">{t('nextactions.f_icon', 'Icon')} <input bind:value={icon} placeholder={t('nextactions.f_icon_ph', 'lucide name, e.g. zap')} class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" /></label>
			<label class="block text-sm">{t('nextactions.f_badge', 'Badge (optional)')} <input bind:value={badge} placeholder={t('nextactions.f_badge_ph', 'eyebrow label, e.g. Getting started')} class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" /></label>
		</div>
	</fieldset>

	<fieldset class="border-border rounded-[var(--radius-card)] border p-3">
		<legend class="text-muted-foreground px-1 text-xs">{t('nextactions.channels', 'Channels (guests: pull-render only)')}</legend>
		<div class="flex flex-wrap gap-3">
			{#each channelsVocab as ch}
				<label class="flex items-center gap-1 text-sm">
					<input type="checkbox" checked={channelsSel.includes(ch)} onchange={() => {
						channelsSel = channelsSel.includes(ch) ? channelsSel.filter((c) => c !== ch) : [...channelsSel, ch];
					}} />
					{ch}
				</label>
			{/each}
		</div>
	</fieldset>

	<fieldset class="border-border rounded-[var(--radius-card)] border p-3">
		<legend class="text-muted-foreground px-1 text-xs">{t('nextactions.conditions', 'Conditions — who sees the suggestion')}</legend>
		<PredicateGroupEditor bind:value={conditionsObj} />
	</fieldset>

	<fieldset class="border-border rounded-[var(--radius-card)] border p-3">
		<legend class="text-muted-foreground px-1 text-xs">{t('nextactions.completion_signal', 'Completion signal — auto-complete when true')}</legend>
		<label class="mb-2 flex items-center gap-2 text-sm">
			<input type="checkbox" checked={completionEnabled} onchange={(e) => {
				completionEnabled = (e.target as HTMLInputElement).checked;
				if (completionEnabled && !completionObj) completionObj = { all: [] };
			}} /> {t('nextactions.enable_completion', 'Enable completion signal')}
		</label>
		{#if completionEnabled && completionObj}
			<PredicateGroupEditor bind:value={completionObj as Record<string, any>} />
		{/if}
	</fieldset>

	<fieldset class="border-border rounded-[var(--radius-card)] border p-3">
		<legend class="text-muted-foreground px-1 text-xs">{t('nextactions.audience', 'Audience')}</legend>
		<AudienceEditor bind:value={audienceObj} />
	</fieldset>

	<fieldset class="border-border rounded-[var(--radius-card)] border p-3">
		<legend class="text-muted-foreground px-1 text-xs">{t('nextactions.frequency', 'Frequency')}</legend>
		<FrequencyEditor bind:value={frequencyObj} />
	</fieldset>

	<div class="flex gap-2">
		<button type="submit" disabled={loading} class="bg-primary text-primary-foreground rounded-[var(--radius)] px-4 py-2 text-sm">{loading ? t('common.saving', 'Saving…') : t('nextactions.save_rule', 'Save rule')}</button>
		<button type="button" onclick={() => goto('/next-actions')} class="text-muted-foreground px-4 py-2 text-sm">{t('common.cancel', 'Cancel')}</button>
	</div>
</form>
{/if}

<ConfirmModal
	bind:open={deleteConfirmOpen}
	title={t('nextactions.delete_title', 'Delete rule')}
	consequence={t('nextactions.delete_confirm', 'Permanently delete rule "{slug}"? Served/dismissed state for users is kept but the suggestion stops rendering.').replace('{slug}', slug)}
	onConfirm={remove}
/>
