<script lang="ts">
	import { api } from '$lib/api';
	import { t, translations } from '$lib/stores/i18n.svelte';
	import type { SystemSetting } from '$lib/api/types';
	import { Dialog } from 'bits-ui';
	import { RotateCcw, Save, Eye, EyeOff, AlertTriangle } from 'lucide-svelte';

	interface Props {
		tenantId: string;
		tenantName?: string;
		open: boolean;
	}

	let { tenantId, tenantName, open = $bindable(false) }: Props = $props();

	let settings = $state<SystemSetting[]>([]);
	let loading = $state(false);
	let saving = $state(false);
	let savingKey = $state<string | null>(null);
	let resettingKey = $state<string | null>(null);
	let message = $state('');
	let messageType = $state<'success' | 'error' | 'info' | ''>('');
	let revealedKeys = $state<Set<string>>(new Set());

	$effect(() => {
		if (open && tenantId) {
			load();
		} else if (!open) {
			settings = [];
			message = '';
			messageType = '';
		}
	});

	async function load() {
		loading = true;
		message = '';
		try {
			const result = await api<SystemSetting[]>(`/control/tenants/${tenantId}/settings`);
			if (result.ok) {
				settings = Array.isArray(result.data) ? result.data.map(normalizeSetting) : [];
			} else {
				message = result.error || t('tenant_settings.load_error', 'Failed to load tenant settings.');
				messageType = 'error';
			}
		} catch {
			message = t('tenant_settings.load_error', 'Failed to load tenant settings.');
			messageType = 'error';
		} finally {
			loading = false;
		}
	}

	function normalizeSetting(s: SystemSetting): SystemSetting {
		return {
			...s,
			value: normalizeValue(s.value),
			default_value: normalizeValue(s.default_value)
		};
	}

	function normalizeValue(v: string): string {
		if (v.length >= 2 && v.startsWith('"') && v.endsWith('"')) {
			const inner = v.slice(1, -1);
			if (inner.startsWith('[') || inner.startsWith('{')) {
				return v;
			}
			return inner;
		}
		return v;
	}

	function validateSetting(s: SystemSetting): string | null {
		const raw = (s.value ?? '').trim();
		if (s.type === 'int' || s.type === 'float') {
			if (raw === '' || Number.isNaN(Number(raw))) {
				return t('tenant_settings.validation_number', '"{key}" must be a valid number.').replace('{key}', s.key);
			}
			const num = Number(raw);
			if (s.type === 'int' && !Number.isInteger(num)) {
				return t('tenant_settings.validation_integer', '"{key}" must be a whole number.').replace('{key}', s.key);
			}
			if (s.min !== undefined && num < s.min) {
				return t('tenant_settings.validation_min', '"{key}" must be at least {min}.').replace('{key}', s.key).replace('{min}', String(s.min));
			}
			if (s.max !== undefined && s.max > 0 && num > s.max) {
				return t('tenant_settings.validation_max', '"{key}" must be at most {max}.').replace('{key}', s.key).replace('{max}', String(s.max));
			}
		}
		if (s.type === 'bool') {
			const unquoted = raw.replace(/^"(.*)"$/, '$1').toLowerCase();
			if (unquoted !== 'true' && unquoted !== 'false') {
				return t('tenant_settings.validation_bool', '"{key}" must be true or false.').replace('{key}', s.key);
			}
		}
		if (raw.startsWith('{') || raw.startsWith('[')) {
			try {
				JSON.parse(raw);
			} catch {
				return t('tenant_settings.validation_json', '"{key}" is not valid JSON.').replace('{key}', s.key);
			}
		}
		return null;
	}

	function isSelect(s: SystemSetting): boolean {
		return !!s.options && s.options.length > 0 && s.type !== 'csv';
	}

	function isTextarea(s: SystemSetting): boolean {
		return s.type === 'json' || s.type === 'csv' || ((s.value ?? '').length > 80 && s.type === 'string');
	}

	function isSensitive(s: SystemSetting): boolean {
		if (s.sensitive) return true;
		return /(secret|password|token|api_key|private)/i.test(s.key);
	}

	function updateValue(key: string, next: string) {
		settings = settings.map((s) => (s.key === key ? { ...s, value: next } : s));
	}

	async function saveAll() {
		const changed = settings.filter((s) => s.value !== s.default_value);
		for (const s of changed) {
			const err = validateSetting(s);
			if (err) {
				message = err;
				messageType = 'error';
				return;
			}
		}
		if (changed.length === 0) {
			message = t('tenant_settings.no_changes', 'No changes to save.');
			messageType = 'info';
			return;
		}
		saving = true;
		message = '';
		const body = Object.fromEntries(changed.map((s) => [s.key, s.value]));
		const result = await api(`/control/tenants/${tenantId}/settings`, { method: 'PATCH', body });
		saving = false;
		if (result.ok) {
			message = t('tenant_settings.save_all_success', '{count} setting(s) saved.').replace('{count}', String(changed.length));
			messageType = 'success';
			await load();
		} else {
			message = result.error || t('tenant_settings.save_error', 'Failed to save settings.');
			messageType = 'error';
		}
	}

	async function saveKey(key: string) {
		const setting = settings.find((s) => s.key === key);
		if (!setting) return;
		const err = validateSetting(setting);
		if (err) {
			message = err;
			messageType = 'error';
			return;
		}
		savingKey = key;
		message = '';
		const result = await api(`/control/tenants/${tenantId}/settings/${key}`, {
			method: 'PATCH',
			body: { value: setting.value }
		});
		savingKey = null;
		if (result.ok) {
			message = t('tenant_settings.save_success', 'Saved "{key}".').replace('{key}', key);
			messageType = 'success';
		} else {
			message = result.error || t('tenant_settings.save_error', 'Failed to save setting.');
			messageType = 'error';
		}
	}

	async function resetKey(key: string) {
		resettingKey = key;
		message = '';
		const result = await api<{ value: string; default_value: string }>(
			`/control/tenants/${tenantId}/settings/${key}/reset`,
			{ method: 'POST' }
		);
		resettingKey = null;
		if (result.ok) {
			const newValue = normalizeValue(result.data?.value ?? '');
			settings = settings.map((s) =>
				s.key === key ? { ...s, value: newValue, default_value: newValue } : s
			);
			message = t('tenant_settings.reset_success', 'Reset "{key}" to default.').replace('{key}', key);
			messageType = 'success';
		} else {
			message = result.error || t('tenant_settings.reset_error', 'Failed to reset setting.');
			messageType = 'error';
		}
	}

	function toggleReveal(key: string) {
		const next = new Set(revealedKeys);
		if (next.has(key)) next.delete(key);
		else next.add(key);
		revealedKeys = next;
	}

	function close() {
		open = false;
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay
			class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50"
		/>
		<Dialog.Content
			class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 flex h-[85vh] w-full max-w-2xl -translate-x-1/2 -translate-y-1/2 flex-col rounded-xl border p-6 focus:outline-none"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-foreground text-lg font-semibold">
				{tenantName ? t('tenant_settings.title_for', 'Settings for {name}').replace('{name}', tenantName) : t('tenant_settings.title', 'Tenant Settings')}
			</Dialog.Title>

			{#if message}
				<div
					class="mt-4 rounded-xl border px-4 py-3 text-sm {messageType === 'error'
						? 'border-destructive bg-destructive/10 text-destructive'
						: messageType === 'success'
							? 'border-emerald-500 bg-emerald-500/10 text-emerald-600'
							: 'border-tertiary bg-tertiary/10 text-tertiary'}"
				>
					{#if messageType === 'error'}<AlertTriangle class="mr-1.5 inline size-4" />{/if}
					{message}
				</div>
			{/if}

			<div class="mt-4 flex-1 overflow-y-auto pr-2">
				{#if loading}
					<div class="text-muted-foreground py-8 text-center text-sm">{t('common.loading', 'Loading...')}</div>
				{:else if settings.length === 0}
					<div class="text-muted-foreground py-8 text-center text-sm">
						{t('tenant_settings.no_settings', 'No settings available for this tenant.')}
					</div>
				{:else}
					<div class="space-y-3">
						{#each settings as setting}
							<div class="border-border-card bg-background-alt rounded-xl border p-4">
								<div class="flex items-start justify-between gap-3">
									<div class="min-w-0 flex-1">
										<p class="text-foreground break-words text-sm font-medium">{setting.key}</p>
										{#if setting.description}
											<p class="text-muted-foreground mt-0.5 text-xs">{setting.description}</p>
										{/if}
										<p class="text-muted-foreground mt-1 text-xs">
											{t('tenant_settings.type', 'Type')}: <span class="font-mono">{setting.type}</span>
											{#if setting.default_value}
												· {t('tenant_settings.default', 'Default')}: <span class="font-mono">{setting.default_value}</span>
											{/if}
										</p>
									</div>
									<div class="flex items-center gap-1">
										<button
											type="button"
											class="text-muted-foreground hover:text-foreground flex size-7 items-center justify-center rounded-lg transition-colors"
											aria-label={t('tenant_settings.save_key', 'Save {key}').replace('{key}', setting.key)}
											disabled={savingKey === setting.key}
											onclick={() => saveKey(setting.key)}
										>
											<Save class="size-4 {savingKey === setting.key ? 'animate-spin' : ''}" />
										</button>
										<button
											type="button"
											class="text-muted-foreground hover:text-foreground flex size-7 items-center justify-center rounded-lg transition-colors"
											aria-label={t('tenant_settings.reset_key', 'Reset {key} to default').replace('{key}', setting.key)}
											disabled={resettingKey === setting.key}
											onclick={() => resetKey(setting.key)}
										>
											<RotateCcw class="size-4 {resettingKey === setting.key ? 'animate-spin' : ''}" />
										</button>
									</div>
								</div>

								<div class="mt-3">
									{#if isSelect(setting)}
										<select
											value={setting.value}
											onchange={(e) => updateValue(setting.key, (e.currentTarget as HTMLSelectElement).value)}
											class="border-border-input bg-background text-foreground w-full rounded-lg border px-3 py-1.5 text-sm transition-all hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
										>
											{#each setting.options ?? [] as opt}
												<option value={opt.value}>{opt.label}</option>
											{/each}
										</select>
									{:else if setting.type === 'bool'}
										<select
											value={setting.value}
											onchange={(e) => updateValue(setting.key, (e.currentTarget as HTMLSelectElement).value)}
											class="border-border-input bg-background text-foreground w-32 rounded-lg border px-3 py-1.5 text-sm transition-all hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
										>
											<option value="true">{t('common.true', 'true')}</option>
											<option value="false">{t('common.false', 'false')}</option>
										</select>
									{:else if setting.type === 'int' || setting.type === 'float'}
										<input
											type="number"
											step={setting.type === 'float' ? '0.01' : '1'}
											value={setting.value}
											oninput={(e) => updateValue(setting.key, (e.currentTarget as HTMLInputElement).value)}
											class="border-border-input bg-background text-foreground w-full rounded-lg border px-3 py-1.5 text-sm transition-all hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
										/>
									{:else if isTextarea(setting)}
										<div class="relative">
											<textarea
												value={setting.value}
												oninput={(e) => updateValue(setting.key, (e.currentTarget as HTMLTextAreaElement).value)}
												rows={setting.type === 'json' ? 4 : 2}
												class="border-border-input bg-background text-foreground w-full rounded-lg border px-3 py-1.5 text-sm font-mono transition-all hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus {isSensitive(setting) && !revealedKeys.has(setting.key) ? 'text-security-disc' : ''}"
											></textarea>
											{#if isSensitive(setting)}
												<button
													type="button"
													class="text-muted-foreground hover:text-foreground absolute right-2 top-2"
													onclick={() => toggleReveal(setting.key)}
													aria-label={t('common.reveal')}
												>
													{#if revealedKeys.has(setting.key)}<EyeOff class="size-4" />{:else}<Eye class="size-4" />{/if}
												</button>
											{/if}
										</div>
									{:else}
										<div class="relative">
											<input
												type={isSensitive(setting) && !revealedKeys.has(setting.key) ? 'password' : 'text'}
												value={setting.value}
												oninput={(e) => updateValue(setting.key, (e.currentTarget as HTMLInputElement).value)}
												class="border-border-input bg-background text-foreground w-full rounded-lg border px-3 py-1.5 text-sm transition-all hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
											/>
											{#if isSensitive(setting)}
												<button
													type="button"
													class="text-muted-foreground hover:text-foreground absolute right-2 top-1/2 -translate-y-1/2"
													onclick={() => toggleReveal(setting.key)}
													aria-label={t('common.reveal')}
												>
													{#if revealedKeys.has(setting.key)}<EyeOff class="size-4" />{:else}<Eye class="size-4" />{/if}
												</button>
											{/if}
										</div>
									{/if}
									{#if setting.type === 'json' || setting.type === 'csv'}
										<p class="text-muted-foreground mt-1 text-xs">
											{setting.type === 'json'
												? t('tenant_settings.json_hint', 'Enter valid JSON.')
												: t('tenant_settings.csv_hint', 'Enter a comma-separated list or JSON array.')}
										</p>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>

			<div class="mt-6 flex justify-end gap-3 border-t border-border-card pt-4">
				<Dialog.Close
					class="border-border-input bg-background hover:bg-muted text-foreground rounded-lg border px-4 py-2 text-sm font-medium transition-colors"
					onclick={close}
				>
					{t('common.close', 'Close')}
				</Dialog.Close>
				<button
					type="button"
					class="bg-foreground text-background rounded-lg px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
					disabled={saving}
					onclick={saveAll}
				>
					{saving ? t('common.saving', 'Saving...') : t('tenant_settings.save_all', 'Save All Changes')}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
