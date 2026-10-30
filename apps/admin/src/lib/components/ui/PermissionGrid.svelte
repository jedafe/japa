<script lang="ts">
	// Shared permission-node picker used by the user-group and role editors.
	// Groups keys by namespace (the segment before the first dot) and marks
	// layered gates with dependency hints so admins see that e.g. list.create
	// is enforced in addition to content.create.
	import { Lock } from 'lucide-svelte';
	import { t } from '$lib/stores/i18n.svelte';

	let {
		permissions = [],
		value = $bindable<string[]>([]),
		isOwner = false
	}: {
		permissions: string[];
		value: string[];
		isOwner: boolean;
	} = $props();

	// Content-type gates enforced inside POST /content on top of the base
	// content.create node (§03: "same pattern as story.create"). Derived from
	// the documented agent.content_type.* family + type-gate seeds.
	const CONTENT_TYPE_GATES = new Set([
		'blog', 'blog_post', 'discussion', 'page', 'comparison', 'guide',
		'event', 'job_listing', 'service_listing', 'story', 'achievement',
		'list', 'wiki', 'video'
	]);

	function hintFor(key: string): string {
		if (key.startsWith('agent.content_type.')) {
			return t('permgrid.requires_agent', 'requires agent.draft_request + content.create');
		}
		const dot = key.indexOf('.');
		if (dot > 0) {
			const ns = key.slice(0, dot);
			if (key.endsWith('.create') && ns !== 'content' && CONTENT_TYPE_GATES.has(ns)) {
				return t('permgrid.requires_content', 'requires content.create');
			}
		}
		return '';
	}

	const groups = $derived.by(() => {
		const m = new Map<string, string[]>();
		for (const p of permissions) {
			const ns = p.includes('.') ? p.slice(0, p.indexOf('.')) : 'system';
			const arr = m.get(ns) ?? [];
			arr.push(p);
			m.set(ns, arr);
		}
		return [...m.entries()].sort((a, b) => a[0].localeCompare(b[0]));
	});

	function isChecked(perm: string): boolean {
		return Array.isArray(value) && value.includes(perm);
	}

	function toggle(perm: string, on: boolean) {
		const current = Array.isArray(value) ? value : [];
		value = on ? [...current, perm] : current.filter((p) => p !== perm);
	}

	function setGroup(keys: string[], on: boolean) {
		const grantable = keys.filter((k) => !(k === '*' && !isOwner));
		const current = new Set(Array.isArray(value) ? value : []);
		for (const k of grantable) {
			if (on) current.add(k);
			else current.delete(k);
		}
		value = [...current];
	}
</script>

<div class="max-h-72 space-y-2 overflow-y-auto">
	{#each groups as [ns, keys] (ns)}
		{@const selected = keys.filter((k) => isChecked(k)).length}
		<details class="rounded-md border" style="border-color: var(--color-border)" open={groups.length <= 8}>
			<summary class="flex cursor-pointer items-center justify-between px-2 py-1.5 text-xs font-medium" style="color: var(--color-foreground)">
				<span class="capitalize">{ns.replace(/_/g, ' ')}</span>
				<span class="flex items-center gap-2">
					<span style="color: var(--color-muted)">{selected}/{keys.length}</span>
					<button
						type="button"
						class="underline"
						style="color: var(--color-primary)"
						onclick={(e) => { e.preventDefault(); setGroup(keys, true); }}
					>{t('common.all', 'all')}</button>
					<button
						type="button"
						class="underline"
						style="color: var(--color-muted)"
						onclick={(e) => { e.preventDefault(); setGroup(keys, false); }}
					>{t('common.none', 'none')}</button>
				</span>
			</summary>
			<div class="grid grid-cols-2 gap-1.5 border-t px-2 py-2" style="border-color: var(--color-border)">
				{#each keys as perm (perm)}
					{@const locked = perm === '*' && !isOwner}
					{@const hint = hintFor(perm)}
					<label class="flex items-start gap-2 text-xs" title={locked ? t('permgrid.wildcard_owner', 'Wildcard grants all access — owner only') : (hint || perm)}>
						<input
							type="checkbox"
							class="mt-0.5 rounded"
							disabled={locked}
							checked={isChecked(perm)}
							onchange={(e) => toggle(perm, e.currentTarget.checked)}
						/>
						<span class="leading-tight">
							{#if perm === '*'}
								<span class="inline-flex items-center gap-1 font-semibold" style="color: var(--color-danger)"><Lock class="size-3" />*</span>
							{:else}
								<span class="break-all" style="color: var(--color-foreground)">{perm}</span>
							{/if}
							{#if hint}
								<span class="block" style="color: var(--color-muted)">({hint})</span>
							{/if}
						</span>
					</label>
				{/each}
			</div>
		</details>
	{/each}
</div>
