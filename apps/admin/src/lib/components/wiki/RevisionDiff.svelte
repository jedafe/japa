<script lang="ts">
	// Renders a fields/mixed wiki revision's ops array as a moderator-facing
	// diff table (§80 community field ops review). For set_value ops the
	// entity's current value is fetched lazily via GET
	// /wiki/entities/:type/:id/fields so the reviewer sees current → proposed;
	// a failed fetch degrades to showing the proposed value alone.
	import { untrack } from 'svelte';
	import { wikiAdminApi, type WikiFieldOp } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type RevisionLike = {
		id?: string;
		kind?: string;
		ops?: WikiFieldOp[] | string | null;
		target_entity_type?: string;
		target_entity_id?: string;
	};

	let { revision }: { revision: RevisionLike } = $props();

	const ops = $derived.by<WikiFieldOp[]>(() => {
		const raw = revision?.ops;
		if (Array.isArray(raw)) return raw;
		if (typeof raw === 'string') {
			try {
				const parsed = JSON.parse(raw);
				return Array.isArray(parsed) ? parsed : [];
			} catch {
				return [];
			}
		}
		return [];
	});

	// Current field values per target entity: "type/id" → field_key → value.
	let currentValues = $state<Record<string, Record<string, unknown>>>({});
	let currentFailed = $state<Record<string, boolean>>({});

	function opTarget(op: WikiFieldOp): { type: string; id: string } {
		return {
			type: op.entity_type || revision?.target_entity_type || '',
			id: op.entity_id || revision?.target_entity_id || ''
		};
	}

	function targetKey(op: WikiFieldOp): string {
		const { type, id } = opTarget(op);
		return type && id ? `${type}/${id}` : '';
	}

	$effect(() => {
		const list = ops;
		const rev = revision;
		untrack(() => {
			const targets = new Map<string, { type: string; id: string }>();
			for (const op of list) {
				if (op.op !== 'set_value') continue;
				const { type, id } = opTarget(op);
				if (type && id) targets.set(`${type}/${id}`, { type, id });
			}
			for (const [key, { type, id }] of targets) {
				if (currentValues[key] !== undefined || currentFailed[key]) continue;
				void wikiAdminApi
					.getEntityFields(type, id)
					.then((res) => {
						if (res.ok) {
							const map: Record<string, unknown> = {};
							for (const g of res.data?.groups ?? []) {
								for (const f of g.fields ?? []) map[f.key] = f.value;
							}
							currentValues[key] = map;
						} else {
							currentFailed[key] = true;
						}
					})
					.catch(() => {
						currentFailed[key] = true;
					});
			}
		});
	});

	function fmt(value: unknown): string {
		if (value === undefined || value === null || value === '') return '—';
		if (typeof value === 'string') return value;
		return JSON.stringify(value);
	}

	function fieldLabel(op: WikiFieldOp): string {
		return op.field_key || op.field?.key || op.group_slug || op.group?.slug || '—';
	}

	function proposedValue(op: WikiFieldOp): string {
		switch (op.op) {
			case 'propose_field':
			case 'upsert_field':
				return op.field
					? `${op.field.label ?? op.field.key ?? ''} (${op.field.field_type ?? '—'})`
					: '—';
			case 'propose_group':
				return op.group
					? `${op.group.name ?? op.group.slug ?? ''} (${op.group.fields?.length ?? 0} ${t('app.wiki.diff_fields', 'fields')})`
					: '—';
			case 'assign_group':
				return `${op.group_slug ?? '—'} → ${op.entity_class ?? '—'}`;
			default:
				return fmt(op.value);
		}
	}
</script>

{#if ops.length === 0}
	<p class="text-sm" style="color: var(--color-muted)">
		{t('app.wiki.diff_no_ops', 'This revision contains no field ops.')}
	</p>
{:else}
	<div class="overflow-x-auto">
		<table class="w-full text-sm" style="color: var(--color-foreground)">
			<thead>
				<tr class="border-b text-start" style="border-color: var(--color-border)">
					<th class="py-2 pe-3 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.diff_op', 'Op')}</th>
					<th class="py-2 pe-3 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.diff_target', 'Target')}</th>
					<th class="py-2 pe-3 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.diff_field', 'Field / Group')}</th>
					<th class="py-2 pe-3 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.diff_value', 'Current → Proposed')}</th>
					<th class="py-2 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.diff_source', 'Source')}</th>
				</tr>
			</thead>
			<tbody>
				{#each ops as op, i (i)}
					{@const tgt = opTarget(op)}
					{@const current = currentValues[targetKey(op)]?.[op.field_key ?? '']}
					<tr class="border-b align-top" style="border-color: var(--color-border)">
						<td class="py-2 pe-3 font-mono text-xs">{op.op}</td>
						<td class="py-2 pe-3">
							{#if tgt.type}
								<span class="font-medium">{tgt.type}</span>
								{#if tgt.id}
									<span class="block font-mono text-xs" style="color: var(--color-muted)" title={tgt.id}>{tgt.id.slice(0, 8)}…</span>
								{/if}
							{:else if op.entity_class}
								<span class="font-medium">{op.entity_class}</span>
							{:else}
								—
							{/if}
						</td>
						<td class="py-2 pe-3 font-mono text-xs">{fieldLabel(op)}</td>
						<td class="py-2 pe-3">
							{#if op.op === 'set_value'}
								{#if current !== undefined}
									<span style="color: var(--color-muted)">{fmt(current)}</span>
									<span class="mx-1" style="color: var(--color-muted)"><span class="inline-block rtl:-scale-x-100">→</span></span>
								{/if}
								<span class="font-medium">{fmt(op.value)}</span>
							{:else}
								{proposedValue(op)}
							{/if}
						</td>
						<td class="py-2">
							{#if op.source_url}
								<a
									href={op.source_url}
									target="_blank"
									rel="noopener noreferrer"
									class="underline"
									style="color: var(--color-primary)">{t('app.wiki.diff_source_link', 'link')}</a>
							{:else}
								—
							{/if}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
