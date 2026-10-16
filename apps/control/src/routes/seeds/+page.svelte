<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { api } from '$lib/api';

	type SeedParam = {
		key: string;
		label: string;
		placeholder?: string;
		type: string;
		required?: boolean;
	};

	type SeedOp = {
		kind: string;
		title: string;
		description: string;
		enqueueable: boolean;
		command?: string;
		params?: SeedParam[];
	};

	let ops = $state<SeedOp[]>([]);
	let loading = $state(true);
	let error = $state('');
	let values = $state<Record<string, Record<string, string | boolean>>>({});
	let busy = $state<Record<string, boolean>>({});
	let notice = $state<Record<string, string>>({});

	onMount(async () => {
		const res = await api<SeedOp[]>('/control/seeds');
		if (res.ok) {
			ops = res.data;
		} else {
			error = res.error;
		}
		loading = false;
	});

	async function enqueue(op: SeedOp) {
		const params = values[op.kind] ?? {};
		for (const p of op.params ?? []) {
			if (p.required && !params[p.key]) {
				notice[op.kind] = `${p.label} is required`;
				return;
			}
		}
		busy[op.kind] = true;
		notice[op.kind] = '';
		const res = await api('/control/seeds/enqueue', {
			method: 'POST',
			body: { kind: op.kind, params },
		});
		busy[op.kind] = false;
		notice[op.kind] = res.ok ? 'Enqueued — track progress under Queue / Cron' : res.error;
	}
</script>

<PageHeader title="Seeds" />

<div class="mx-auto w-full max-w-3xl space-y-4 p-4">
	<p class="text-sm text-[var(--color-text-subtle)]">
		Trigger data seeding and enrichment jobs. Enqueued ops run in the
		worker — watch Queue / Cron for progress.
	</p>

	{#if loading}
		<p class="text-sm text-[var(--color-text-subtle)]">Loading…</p>
	{:else if error}
		<p class="text-sm text-red-500">{error}</p>
	{:else}
		{#each ops as op (op.kind)}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="text-base font-semibold">{op.title}</h2>
				<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{op.description}</p>

				{#if op.command}
					<code class="mt-2 block rounded bg-[var(--color-bg)] px-3 py-2 text-xs">{op.command}</code>
				{/if}

				{#if op.enqueueable}
					<div class="mt-3 space-y-2">
						{#each op.params ?? [] as param (param.key)}
							{#if param.type === 'checkbox'}
								<label class="flex items-center gap-2 text-sm">
									<input
										type="checkbox"
										checked={!!values[op.kind]?.[param.key]}
										onchange={(e) => {
											values[op.kind] ??= {};
											values[op.kind][param.key] = (e.target as HTMLInputElement).checked;
										}}
									/>
									{param.label}
								</label>
							{:else}
								<input
									class="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm"
									type={param.type === 'number' ? 'number' : 'text'}
									placeholder={param.placeholder ?? param.label}
									value={values[op.kind]?.[param.key] ?? ''}
									oninput={(e) => {
										values[op.kind] ??= {};
										values[op.kind][param.key] = (e.target as HTMLInputElement).value;
									}}
								/>
							{/if}
						{/each}
						<button
							class="rounded bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
							disabled={busy[op.kind]}
							onclick={() => enqueue(op)}
						>
							{busy[op.kind] ? 'Enqueuing…' : 'Enqueue job'}
						</button>
						{#if notice[op.kind]}
							<p class="text-sm text-[var(--color-text-subtle)]">{notice[op.kind]}</p>
						{/if}
					</div>
				{/if}
			</div>
		{/each}
	{/if}
</div>
