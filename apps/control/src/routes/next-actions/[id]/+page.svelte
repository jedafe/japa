<!-- §122 Next-Action rule editor -->
<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { getRule, createRule, updateRule, deleteRule, type NextActionRule } from '$lib/api/nextactions';
	import PredicateGroupEditor from '$lib/components/PredicateGroupEditor.svelte';

	const id = $derived(page.params.id ?? '');
	const isNew = $derived(id === 'new');

	let slug = $state('');
	let name = $state('');
	let description = $state('');
	let enabled = $state(false);
	let priority = $state(100);
	let title = $state('');
	let body = $state('');
	let ctaLabel = $state('');
	let ctaUrl = $state('');
	let channelsSel = $state<string[]>([]);
	let upsell = $state(false);
	let icon = $state('');
	let validFrom = $state('');
	let validUntil = $state('');
	let audienceJson = $state('{"user_class":["regular"]}');
	let conditionsObj = $state<Record<string, any>>({ all: [] });
	let completionObj = $state<Record<string, any> | null>(null);
	let completionEnabled = $state(false);
	let frequencyJson = $state('{"mode":"recurring","max_impressions":5,"cooldown_hours":72,"snooze_default_hours":168}');

	let loading = $state(false);
	let message = $state('');
	let messageType = $state<'success' | 'error' | 'info'>('info');

	const CHANNELS = ['popup', 'announcement_bar', 'sidebar', 'in_article', 'notification', 'push', 'email'];

	$effect(() => {
		if (isNew) return;
		getRule(id).then((r) => {
			if (!r.ok || !r.data) { message = r.ok ? 'Rule not found' : r.error; messageType = 'error'; return; }
			const d = r.data as NextActionRule;
			slug = d.slug; name = d.name; description = d.description;
			enabled = d.enabled; priority = d.priority;
			upsell = d.upsell ?? false;
			title = d.payload.title; body = d.payload.body;
			ctaLabel = d.payload.cta_label; ctaUrl = d.payload.cta_url;
			icon = d.payload.icon ?? '';
			validFrom = toLocalInput(d.valid_from);
			validUntil = toLocalInput(d.valid_until);
			channelsSel = d.channels ?? [];
			audienceJson = JSON.stringify(d.audience ?? {}, null, 2);
			conditionsObj = (d.conditions && typeof d.conditions === 'object' ? d.conditions : { all: [] }) as Record<string, any>;
			completionObj = d.completion_signal && typeof d.completion_signal === 'object' ? d.completion_signal : null;
			completionEnabled = !!d.completion_signal;
			frequencyJson = JSON.stringify(d.frequency ?? {}, null, 2);
		});
	});

	function parseJson(raw: string, label: string, required = true) {
		if (!required && raw.trim() === '') return null;
		try { return JSON.parse(raw); } catch { throw new Error(`${label} is not valid JSON`); }
	}

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
		try {
			const payload = {
				slug, name, description, enabled, priority, upsell,
				audience: parseJson(audienceJson, 'audience'),
				conditions: conditionsObj,
				payload: { title, body, cta_label: ctaLabel, cta_url: ctaUrl, icon },
				channels: channelsSel,
				completion_signal: completionEnabled ? completionObj : null,
				frequency: parseJson(frequencyJson, 'frequency'),
				valid_from: toApiTs(validFrom),
				valid_until: toApiTs(validUntil)
			};
			loading = true;
			const r = isNew ? await createRule(payload) : await updateRule(id, payload);
			loading = false;
			if (!r.ok) { message = r.error || 'Save failed'; messageType = 'error'; return; }
			message = 'Saved'; messageType = 'success';
			if (isNew) goto('/next-actions');
		} catch (e) {
			loading = false;
			message = e instanceof Error ? e.message : 'Save failed';
			messageType = 'error';
		}
	}

	async function remove() {
		if (!confirm('Delete this rule?')) return;
		const r = await deleteRule(id);
		if (r.ok) goto('/next-actions');
		else { message = r.error || 'Delete failed'; messageType = 'error'; }
	}
</script>

<PageHeader title={isNew ? 'New next-action rule' : `Edit rule — ${slug}`} description="§122 suggestion rule (JSON fields validated server-side)" />

{#if !isNew}
	<div class="mb-3">
		<button onclick={remove} class="border-destructive text-destructive rounded-[var(--radius)] border px-3 py-1.5 text-sm">Delete rule</button>
	</div>
{/if}

{#if message}<AlertBanner variant={messageType} {message} />{/if}

<form onsubmit={(e) => { e.preventDefault(); void save(); }} class="mt-4 max-w-3xl space-y-4">
	<div class="grid grid-cols-2 gap-4">
		<label class="block text-sm">Slug
			<input bind:value={slug} required disabled={!isNew} class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5 font-mono text-xs" />
		</label>
		<label class="block text-sm">Name
			<input bind:value={name} required class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" />
		</label>
		<label class="block text-sm">Priority (lower = first)
			<input bind:value={priority} type="number" class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" />
		</label>
		<label class="mt-6 flex items-center gap-2 text-sm">
			<input bind:checked={enabled} type="checkbox" /> Enabled
		</label>
		<label class="mt-6 flex items-center gap-2 text-sm" title="Counts toward nextactions.max_upsell_per_day">
			<input bind:checked={upsell} type="checkbox" /> Upsell
		</label>
	</div>
	<div class="grid grid-cols-2 gap-4">
		<label class="block text-sm">Valid from (optional)
			<input bind:value={validFrom} type="datetime-local" class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" />
		</label>
		<label class="block text-sm">Valid until (optional)
			<input bind:value={validUntil} type="datetime-local" class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" />
		</label>
	</div>
	<label class="block text-sm">Description
		<input bind:value={description} class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" />
	</label>

	<fieldset class="border-border rounded-[var(--radius-card)] border p-3">
		<legend class="text-muted-foreground px-1 text-xs">Payload</legend>
		<div class="grid grid-cols-2 gap-3">
			<label class="block text-sm">Title <input bind:value={title} required class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" /></label>
			<label class="block text-sm">CTA label <input bind:value={ctaLabel} class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" /></label>
			<label class="block text-sm">CTA URL <input bind:value={ctaUrl} required placeholder="/pricing" class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5 font-mono text-xs" /></label>
			<label class="block text-sm">Body <input bind:value={body} class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" /></label>
			<label class="block text-sm">Icon <input bind:value={icon} placeholder="lucide name, e.g. zap" class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5" /></label>
		</div>
	</fieldset>

	<fieldset class="border-border rounded-[var(--radius-card)] border p-3">
		<legend class="text-muted-foreground px-1 text-xs">Channels (guests: pull-render only)</legend>
		<div class="flex flex-wrap gap-3">
			{#each CHANNELS as ch}
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
		<legend class="text-muted-foreground px-1 text-xs">Conditions — who sees the suggestion</legend>
		<PredicateGroupEditor bind:value={conditionsObj} />
	</fieldset>

	<fieldset class="border-border rounded-[var(--radius-card)] border p-3">
		<legend class="text-muted-foreground px-1 text-xs">Completion signal — auto-complete when true</legend>
		<label class="mb-2 flex items-center gap-2 text-sm">
			<input type="checkbox" checked={completionEnabled} onchange={(e) => {
				completionEnabled = (e.target as HTMLInputElement).checked;
				if (completionEnabled && !completionObj) completionObj = { all: [] };
			}} /> Enable completion signal
		</label>
		{#if completionEnabled && completionObj}
			<PredicateGroupEditor bind:value={completionObj as Record<string, any>} />
		{/if}
	</fieldset>

	{#each [
		['audience', 'audienceJson'],
		['frequency', 'frequencyJson']
	] as [label, key]}
		<label class="block text-sm">{label}
			<textarea
				rows={5}
				class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5 font-mono text-xs"
				value={key === 'audienceJson' ? audienceJson : frequencyJson}
				oninput={(e) => {
					const v = (e.target as HTMLTextAreaElement).value;
					if (key === 'audienceJson') audienceJson = v;
					else frequencyJson = v;
				}}
			></textarea>
		</label>
	{/each}

	<div class="flex gap-2">
		<button type="submit" disabled={loading} class="bg-primary text-primary-foreground rounded-[var(--radius)] px-4 py-2 text-sm">{loading ? 'Saving…' : 'Save rule'}</button>
		<button type="button" onclick={() => goto('/next-actions')} class="text-muted-foreground px-4 py-2 text-sm">Cancel</button>
	</div>
</form>
