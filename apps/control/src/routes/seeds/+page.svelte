<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	type SeedParam = {
		key: string;
		label: string;
		placeholder?: string;
		type: string; // text | number | checkbox | select
		required?: boolean;
		options?: string[];
	};

	type SeedOp = {
		kind: string;
		title: string;
		description: string;
		enqueueable: boolean;
		command?: string;
		params?: SeedParam[];
	};

	type Notice = { variant: 'success' | 'error'; text: string };

	let ops = $state<SeedOp[]>([]);
	let loading = $state(true);
	let error = $state('');
	let values = $state<Record<string, Record<string, string | boolean>>>({});
	let busy = $state<Record<string, boolean>>({});
	let notice = $state<Record<string, Notice>>({});

	onMount(async () => {
		const res = await api<SeedOp[]>('/control/seeds');
		if (res.ok) {
			ops = res.data;
		} else {
			error = res.error;
		}
		loading = false;
	});

	function setNotice(kind: string, n: Notice) {
		notice[kind] = n;
	}

	async function enqueue(op: SeedOp) {
		const params = values[op.kind] ?? {};
		for (const p of op.params ?? []) {
			if (p.required && !params[p.key]) {
				setNotice(op.kind, {
					variant: 'error',
					text: t('seeds.param_required', '{label} is required').replace('{label}', p.label)
				});
				return;
			}
		}
		busy[op.kind] = true;
		delete notice[op.kind];
		const res = await api<{ status: string; task: string }>('/control/seeds/enqueue', {
			method: 'POST',
			body: { kind: op.kind, params }
		});
		busy[op.kind] = false;
		setNotice(
			op.kind,
			res.ok
				? {
						variant: 'success',
						text: res.data.task
							? `${t('seeds.enqueued', 'Job enqueued.')} (${res.data.task})`
							: t('seeds.enqueued', 'Job enqueued.')
					}
				: { variant: 'error', text: res.error }
		);
	}
</script>

<PageHeader title={t('seeds.title', 'Seeds')} />

<div class="mx-auto w-full max-w-3xl space-y-4 p-4">
	<p class="text-sm text-muted-foreground">
		{t(
			'seeds.intro',
			'Trigger data seeding and enrichment jobs. Enqueued ops run in the worker — watch Queue / Cron for progress.'
		)}
	</p>

	{#if loading}
		<p class="text-sm text-muted-foreground">{t('seeds.loading', 'Loading…')}</p>
	{:else if error}
		<AlertBanner variant="error" message={error} />
	{:else if ops.length === 0}
		<div class="rounded-9px border border-border-input bg-background-alt p-6 text-center">
			<p class="text-sm text-muted-foreground">
				{t('seeds.empty', 'No seed operations are configured.')}
			</p>
		</div>
	{:else}
		{#each ops as op (op.kind)}
			<div class="rounded-9px border border-border-input bg-background-alt p-4">
				<h2 class="text-base font-semibold text-foreground">{op.title}</h2>
				<p class="mt-1 text-sm text-muted-foreground">{op.description}</p>

				{#if op.command}
					<code class="mt-2 block rounded-5px bg-muted px-3 py-2 font-mono text-xs text-foreground">{op.command}</code>
				{/if}

				{#if op.enqueueable}
					<form
						class="mt-3 space-y-2"
						onsubmit={(e) => {
							e.preventDefault();
							enqueue(op);
						}}
					>
						{#each op.params ?? [] as param (param.key)}
							{@const inputId = `seed-${op.kind}-${param.key}`}
							{#if param.type === 'checkbox'}
								<label for={inputId} class="flex items-center gap-2 text-sm text-foreground">
									<input
										id={inputId}
										type="checkbox"
										checked={!!values[op.kind]?.[param.key]}
										onchange={(e) => {
											values[op.kind] ??= {};
											values[op.kind][param.key] = (e.target as HTMLInputElement).checked;
										}}
									/>
									{param.label}
								</label>
							{:else if param.type === 'select' && param.options?.length}
								<div>
									<label for={inputId} class="mb-1 block text-sm text-foreground">
										{param.label}
										{#if param.required}
											<span class="text-destructive" aria-hidden="true">*</span>
										{/if}
									</label>
									<AutocompleteSelect
										id={inputId}
										value={String(values[op.kind]?.[param.key] ?? '')}
										onValueChange={(v) => {
										values[op.kind] ??= {};
										values[op.kind][param.key] = v;
										}}
										placeholder={param.required ? t('seeds.select_placeholder', 'Select…') : t('seeds.select_default', '(default)')}
										options={[
										...(param.required ? [] : [{ value: '', label: t('seeds.select_default', '(default)') }]),
										...param.options.map((o) => ({ value: o, label: o }))
										]}
									/>
								</div>
							{:else}
								<div>
									<label for={inputId} class="mb-1 block text-sm text-foreground">
										{param.label}
										{#if param.required}
											<span class="text-destructive" aria-hidden="true">*</span>
										{/if}
									</label>
									<input
										id={inputId}
										class="border-border-input bg-background text-foreground w-full rounded-5px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover"
										type={param.type === 'number' ? 'number' : 'text'}
										required={param.required}
										placeholder={param.placeholder ?? param.label}
										value={String(values[op.kind]?.[param.key] ?? '')}
										oninput={(e) => {
											values[op.kind] ??= {};
											values[op.kind][param.key] = (e.target as HTMLInputElement).value;
										}}
									/>
								</div>
							{/if}
						{/each}
						<button
							type="submit"
							class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
							disabled={busy[op.kind]}
						>
							{busy[op.kind]
								? t('seeds.enqueuing', 'Enqueuing…')
								: t('seeds.enqueue', 'Enqueue job')}
						</button>
						{#if notice[op.kind]}
							<AlertBanner variant={notice[op.kind].variant} message={notice[op.kind].text} />
							{#if notice[op.kind].variant === 'success'}
								<p class="text-sm text-muted-foreground">
									{t('seeds.track_prefix', 'Track progress under')}
									<a href="/queue" class="text-foreground underline">{t('seeds.queue_link', 'Queue')}</a>
									/
									<a href="/cron" class="text-foreground underline">{t('seeds.cron_link', 'Cron')}</a>.
								</p>
							{/if}
						{/if}
					</form>
				{/if}
			</div>
		{/each}
	{/if}
</div>
