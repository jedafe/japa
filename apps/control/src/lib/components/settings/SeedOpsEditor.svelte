<script lang="ts">
	// SeedOpsEditor — structured editor for the control.seed_ops system_setting
	// (JSON array of SeedOp). Lets operators add/edit seed ops without writing
	// raw JSON: kind, title, description, enqueueable toggle, CLI command,
	// task_type select populated from GET /control/seeds/task-types (the
	// allowlist — Rule 16: no hardcoded task list), optional static payload
	// JSON, and a param list editor.
	//
	// Ref: internal/control/handlers_seeds.go (SeedOp/SeedParam contract)
	import { onMount } from 'svelte';
	import { Plus, Trash2, ChevronDown, ChevronRight } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	interface SeedParam {
		key: string;
		label: string;
		placeholder?: string;
		type: string;
		required?: boolean;
		options?: string[];
	}

	interface SeedOp {
		kind: string;
		title: string;
		description: string;
		enqueueable: boolean;
		task_type?: string;
		command?: string;
		static?: Record<string, unknown>;
		params?: SeedParam[];
		// Editor-local UI state — never serialized (emit writes known fields only)
		open?: boolean;
		staticRaw?: string;
	}

	let {
		value,
		disabled = false,
		onchange
	}: {
		value: string;
		disabled?: boolean;
		onchange: (next: string) => void;
	} = $props();

	interface TaskSchema {
		type: string;
		params: SeedParam[];
	}

	let ops = $state<SeedOp[]>([]);
	let taskTypes = $state<TaskSchema[]>([]);
	let lastEmitted = '';

	const paramTypes = ['text', 'number', 'checkbox', 'select'];

	$effect(() => {
		if (value === lastEmitted) return;
		// Build from the local parse — reading `ops` here would make it an
		// effect dependency that this same effect writes → infinite loop.
		const parsed = parseOps(value);
		for (const op of parsed) {
			// staticRaw holds the raw static-payload text so an in-progress
			// (temporarily invalid) JSON edit isn't lost on every keystroke.
			op.staticRaw = op.static ? JSON.stringify(op.static) : '';
		}
		ops = parsed;
	});

	onMount(async () => {
		const res = await api<TaskSchema[]>('/control/seeds/task-types');
		if (res.ok) taskTypes = res.data;
	});

	function schemaFor(op: SeedOp): SeedParam[] {
		return taskTypes.find((t) => t.type === op.task_type)?.params ?? [];
	}

	// Picking a schema param key fills label/type/required/options from the
	// backend-declared contract; a key absent from the schema stays editable.
	function pickParamKey(param: SeedParam, op: SeedOp, key: string) {
		param.key = key;
		const decl = schemaFor(op).find((p) => p.key === key);
		if (decl) {
			param.label = decl.label;
			param.type = decl.type;
			param.required = decl.required;
			param.options = decl.options;
			param.placeholder = decl.placeholder;
		}
		emit();
	}

	function parseOps(v: string): SeedOp[] {
		try {
			const arr = JSON.parse(v || '[]');
			return Array.isArray(arr) ? arr : [];
		} catch {
			return [];
		}
	}

	function emit() {
		// Serialize only populated fields so the stored JSON stays clean.
		const clean = ops.map((op) => {
			const o: SeedOp = {
				kind: op.kind,
				title: op.title,
				description: op.description,
				enqueueable: !!op.enqueueable
			};
			if (op.task_type) o.task_type = op.task_type;
			if (op.command) o.command = op.command;
			try {
				const s = JSON.parse(op.staticRaw || 'null');
				if (s && typeof s === 'object') o.static = s;
			} catch {
				// keep last valid static; invalid in-flight text is skipped
			}
			const params = (op.params ?? []).filter((p) => p.key);
			if (params.length) o.params = params;
			return o;
		});
		lastEmitted = JSON.stringify(clean);
		onchange(lastEmitted);
	}

	function addOp() {
		ops = [
			...ops,
			{ kind: '', title: '', description: '', enqueueable: false, params: [], open: true, staticRaw: '' }
		];
		emit();
	}

	function removeOp(i: number) {
		ops = ops.filter((_, idx) => idx !== i);
		emit();
	}

	function addParam(op: SeedOp) {
		op.params = [...(op.params ?? []), { key: '', label: '', type: 'text' }];
		emit();
	}

	function removeParam(op: SeedOp, i: number) {
		op.params = (op.params ?? []).filter((_, idx) => idx !== i);
		emit();
	}

	function setOptionsCsv(param: SeedParam, csv: string) {
		const list = csv.split(',').map((s) => s.trim()).filter(Boolean);
		param.options = list.length ? list : undefined;
		emit();
	}

	const inputCls =
		'border-border-input bg-background text-foreground w-full rounded-5px border px-2 py-1.5 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover disabled:opacity-50';
	const labelCls = 'mb-1 block text-xs text-muted-foreground';
</script>

<div class="w-full space-y-2">
	{#each ops as op, i (i)}
		<div class="rounded-9px border border-border-input bg-background-alt p-3">
			<div class="flex items-center gap-2">
				<button
					type="button"
					class="text-muted-foreground"
					onclick={() => (op.open = !op.open)}
					aria-label={op.open ? 'Collapse' : 'Expand'}
				>
					{#if op.open}<ChevronDown class="size-4" />{:else}<ChevronRight class="size-4 rtl:-scale-x-100" />{/if}
				</button>
				<input
					class="{inputCls} flex-1 font-medium"
					placeholder={t('seeds_editor.title_ph', 'Op title')}
					bind:value={op.title}
					oninput={emit}
					{disabled}
				/>
				<label class="text-muted-foreground flex shrink-0 items-center gap-1.5 text-xs">
					<input type="checkbox" bind:checked={op.enqueueable} onchange={emit} {disabled} />
					{t('seeds_editor.enqueueable', 'Enqueueable')}
				</label>
				<button
					type="button"
					class="text-destructive shrink-0 p-1 disabled:opacity-30"
					onclick={() => removeOp(i)}
					{disabled}
					aria-label={t('seeds_editor.remove_op', 'Remove op')}
				>
					<Trash2 class="size-4" />
				</button>
			</div>

			{#if op.open}
				<div class="mt-3 space-y-3 border-t border-border-input pt-3">
					<div class="grid grid-cols-2 gap-2">
						<div>
							<label class={labelCls} for="seed-op-kind-{i}">{t('seeds_editor.kind', 'Kind (unique id)')}</label>
							<p class="text-muted-foreground mt-1 text-xs">{t('seeds_editor.kind_hint', 'Unique identifier for this seed operation.')}</p>
							<input id="seed-op-kind-{i}" class="{inputCls} font-mono" placeholder="my_op" bind:value={op.kind} oninput={emit} {disabled} />
						</div>
						<div>
							<label class={labelCls} for="seed-op-task-{i}">{t('seeds_editor.task_type', 'Task type')}</label>
							<p class="text-muted-foreground mt-1 text-xs">{t('seeds_editor.task_hint', 'The task type this op runs.')}</p>
							<select id="seed-op-task-{i}" class={inputCls} bind:value={op.task_type} onchange={emit} {disabled}>
								<option value="">{t('seeds_editor.no_task', '— none (display/CLI only) —')}</option>
								{#each taskTypes as tt (tt.type)}
									<option value={tt.type}>{tt.type}</option>
								{/each}
							</select>
						</div>
					</div>
					<div>
						<label class={labelCls} for="seed-op-desc-{i}">{t('seeds_editor.description', 'Description')}</label>
						<p class="text-muted-foreground mt-1 text-xs">{t('seeds_editor.desc_hint', 'What this op does — for admin reference.')}</p>
						<input id="seed-op-desc-{i}" class={inputCls} bind:value={op.description} oninput={emit} {disabled} />
					</div>
					<div class="grid grid-cols-2 gap-2">
						<div>
							<label class={labelCls} for="seed-op-cmd-{i}">{t('seeds_editor.command', 'CLI command (optional)')}</label>
							<p class="text-muted-foreground mt-1 text-xs">{t('seeds_editor.cmd_hint', 'Optional CLI command to run instead.')}</p>
							<input id="seed-op-cmd-{i}" class="{inputCls} font-mono" placeholder="go run ./cmd/..." bind:value={op.command} oninput={emit} {disabled} />
						</div>
						<div>
							<label class={labelCls} for="seed-op-static-{i}">{t('seeds_editor.static', 'Static payload JSON (optional)')}</label>
							<p class="text-muted-foreground mt-1 text-xs">{t('seeds_editor.static_hint', 'Optional static JSON payload.')}</p>
							<input
								id="seed-op-static-{i}"
								class="{inputCls} font-mono"
								placeholder={'{"version":"1"}'}
								value={op.staticRaw ?? ''}
								oninput={(e) => {
									op.staticRaw = (e.target as HTMLInputElement).value;
									emit();
								}}
								{disabled}
							/>
						</div>
					</div>

					{#if op.enqueueable}
						<div>
							<div class="mb-1 flex items-center justify-between">
								<span class="text-xs text-muted-foreground">{t('seeds_editor.params', 'Params')}</span>
								<button
									type="button"
									class="text-foreground flex items-center gap-1 text-xs hover:opacity-80 disabled:opacity-30"
									onclick={() => addParam(op)}
									{disabled}
								>
									<Plus class="size-3" /> {t('seeds_editor.add_param', 'Add param')}
								</button>
							</div>
							{#each op.params ?? [] as param, pi (pi)}
								{@const declKeys = schemaFor(op).map((p) => p.key)}
								<div class="mb-1.5 grid grid-cols-[1fr_1fr_90px_64px_1fr_28px] items-center gap-1.5">
									{#if declKeys.length}
										<select
											class="{inputCls} font-mono"
											value={param.key}
											onchange={(e) => pickParamKey(param, op, (e.target as HTMLSelectElement).value)}
											{disabled}
										>
											<option value="">{t('seeds_editor.pick_key', 'pick param…')}</option>
											{#each declKeys as k (k)}
												<option value={k} selected={param.key === k}>{k}</option>
											{/each}
											{#if param.key && !declKeys.includes(param.key)}
												<option value={param.key} selected>{param.key} (custom)</option>
											{/if}
										</select>
									{:else}
										<input class="{inputCls} font-mono" placeholder="key" bind:value={param.key} oninput={emit} {disabled} />
									{/if}
									<input class={inputCls} placeholder="label" bind:value={param.label} oninput={emit} {disabled} />
									<select class={inputCls} bind:value={param.type} onchange={emit} {disabled}>
										{#each paramTypes as pt (pt)}<option value={pt}>{pt}</option>{/each}
									</select>
									<label class="text-muted-foreground flex items-center justify-center gap-1 text-xs">
										<input type="checkbox" bind:checked={param.required} onchange={emit} {disabled} />
										req
									</label>
									{#if param.type === 'select'}
										<input
											class={inputCls}
											placeholder={t('seeds_editor.options_csv', 'options, csv')}
											value={(param.options ?? []).join(', ')}
											oninput={(e) => setOptionsCsv(param, (e.target as HTMLInputElement).value)}
											{disabled}
										/>
									{:else}
										<input class={inputCls} placeholder="placeholder" bind:value={param.placeholder} oninput={emit} {disabled} />
									{/if}
									<button type="button" class="text-destructive disabled:opacity-30" onclick={() => removeParam(op, pi)} {disabled} aria-label={t('seeds_editor.remove_param', 'Remove param')}>
										<Trash2 class="size-3.5" />
									</button>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{/if}
		</div>
	{/each}

	<button
		type="button"
		class="border-border-input bg-background hover:bg-muted text-foreground rounded-9px flex w-full items-center justify-center gap-1.5 border px-3 py-2 text-sm font-medium transition-colors disabled:opacity-50"
		onclick={addOp}
		{disabled}
	>
		<Plus class="size-4" /> {t('seeds_editor.add_op', 'Add seed op')}
	</button>
	<p class="text-xs text-muted-foreground">
		{t('seeds_editor.hint', 'Ops with an allowlisted task_type get an Enqueue button on the Seeds page; others show as display/command cards.')}
	</p>
</div>
