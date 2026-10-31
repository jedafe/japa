<script lang="ts">
	import { untrack } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { t, i18n } from '$lib/i18n';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import { Plus, Trash2, Copy, Check, AlertTriangle, Pencil, RefreshCw, Shield, X } from 'lucide-svelte';
	import type { PageData, ActionData } from './$types';
	import type { ApiKeySummary } from '$lib/types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	// Create-key dialog state
	let showCreate = $state(false);
	let keyName = $state('');
	let rateLimitTier = $state<ApiKeySummary['rate_limit_tier']>('basic');
	let selectedScopes = $state<string[]>([]);
	let creating = $state(false);
	let createError = $state('');

	// Newly created/rotated key — shown once (Ref: §47 ROADMAP 06.02)
	let newKey = $state('');
	let newKeyRotated = $state(false);
	let copiedNew = $state(false);

	// Single-dialog state — only one of these can be open at a time, so a
	// shared error field renders inside whichever dialog is open.
	type DialogKind = 'rename' | 'rotate' | 'ips' | 'revoke';
	let dialog = $state<DialogKind | null>(null);
	let dialogKey = $state<ApiKeySummary | null>(null);
	let dialogError = $state('');
	let dialogWorking = $state(false);
	let renameName = $state('');
	let ipCidr = $state('');
	let ipDesc = $state('');

	// Copied prefix indicator
	let copiedId = $state<string | null>(null);

	// The backend limit counts only active keys (internal/publicapi enforces
	// api.keys.max_per_user against status='active') — revoked keys don't count.
	let activeKeyCount = $derived(data.keys.filter((k) => k.status === 'active').length);
	let atLimit = $derived(activeKeyCount >= data.maxKeys);

	// Selectable scopes = the self-service catalog only (GET /developer/scopes —
	// Rule 16). Admin-granted scopes on existing keys (write scopes, 'mcp', '*')
	// render in the help list but must NOT be pickable — the backend rejects
	// them on self-service create (§47).
	let catalogScopes = $derived(data.scopes ?? []);
	let scopeOptions = $derived.by(() => {
		const seen = new Map<string, { value: string; label: string; description: string }>();
		for (const s of catalogScopes) {
			seen.set(s.value, { value: s.value, label: s.label, description: s.description });
		}
		for (const k of data.keys) {
			for (const s of k.scopes ?? []) {
				if (!seen.has(s)) {
					seen.set(s, {
						value: s,
						label: s,
						description: t('api.scope_admin_granted', 'Admin-granted scope')
					});
				}
			}
		}
		return [...seen.values()];
	});

	// Scope multi-select: Autocomplete is single-select, so each pick appends
	// to selectedScopes (rendered as removable chips) and the hidden input
	// carries the comma-joined contract the action expects.
	const scopeFetcher = $derived(
		(q: string): Promise<AutocompleteItem[]> => {
			const needle = q.trim().toLowerCase();
			return Promise.resolve(
				catalogScopes
					.filter((s) => !selectedScopes.includes(s.value))
					.filter(
						(s) =>
							!needle ||
							s.value.toLowerCase().includes(needle) ||
							s.label.toLowerCase().includes(needle) ||
							s.description.toLowerCase().includes(needle)
					)
					.map((s) => ({ id: s.value, label: s.label, sublabel: s.value }))
			);
		}
	);

	function addScope(item: AutocompleteItem) {
		if (!selectedScopes.includes(item.id)) selectedScopes = [...selectedScopes, item.id];
	}
	function removeScope(s: string) {
		selectedScopes = selectedScopes.filter((x) => x !== s);
	}

	async function copyText(text: string, id: string) {
		try {
			await navigator.clipboard.writeText(text);
			copiedId = id;
			setTimeout(() => (copiedId = null), 2000);
		} catch {
			// clipboard not available
		}
	}

	function openCreate() {
		showCreate = true;
		keyName = '';
		rateLimitTier = 'basic';
		selectedScopes = [];
		createError = '';
		newKey = '';
	}

	function openDialog(kind: DialogKind, key: ApiKeySummary) {
		dialog = kind;
		dialogKey = key;
		dialogError = '';
		dialogWorking = false;
		if (kind === 'rename') renameName = key.name;
		if (kind === 'ips') {
			ipCidr = '';
			ipDesc = '';
		}
	}

	function closeDialog() {
		dialog = null;
		dialogKey = null;
		dialogError = '';
	}

	function statusLabel(status: string): string {
		return t(`api.status_${status}`, status);
	}

	function expiryLabel(key: ApiKeySummary): string | null {
		if (!key.expires_at) return null;
		const at = new Date(key.expires_at);
		if (Number.isNaN(at.getTime())) return null;
		return t('api.expires_on', 'Expires {date}', {
			date: at.toLocaleDateString(i18n.locale, { year: 'numeric', month: 'short', day: 'numeric' })
		});
	}

	// Handle form action results
	$effect(() => {
		if (form?.created) {
			newKey = form.created.key;
			newKeyRotated = false;
			copiedNew = false;
			showCreate = false;
			invalidateAll();
		}
		if (form?.rotated) {
			newKey = form.rotated.key;
			newKeyRotated = true;
			copiedNew = false;
			closeDialog();
			invalidateAll();
		}
		if (form?.revoked || form?.renamed) {
			closeDialog();
			invalidateAll();
		}
		if (form?.ipAdded || form?.ipRemoved) {
			// Keep the allowlist dialog open — invalidateAll refreshes the
			// entry list in place so the user sees the change immediately.
			ipCidr = '';
			ipDesc = '';
			invalidateAll();
		}
		if (form?.error) {
			if (showCreate) createError = form.error;
			else if (dialog) dialogError = form.error;
			else createError = form.error;
		}
	});
</script>

<svelte:window
	onkeydown={(e) => {
		if (e.key !== 'Escape') return;
		if (showCreate) showCreate = false;
		else if (dialog) closeDialog();
	}}
/>

<svelte:head>
	<title>{data.meta.title}</title>
	<meta name="description" content={data.meta.description} />
	<link rel="canonical" href={data.meta.ogUrl} />
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<AppShell
	nav={data.nav}
	accessibleAudiences={data.accessibleAudiences}
	logoUrl={(data.siteSettings?.["appearance.logo_url"] as string | undefined) ?? ""}
	footerMenu={data.footerMenu}
	rightSidebarWidgets={data.rightSidebarWidgets}
	leftSidebarWidgets={data.leftSidebarWidgets}
	activeAudience="api"
	user={data.user}
	collapsed={collapsed}
	onToggleCollapse={toggleCollapse}
>
	<section class="keys-page">
		<header class="keys-header">
			<a href="/docs/api" class="back-link">← {t('audience.api')}</a>
			<h1>{t('api.keys_title', 'API Keys')}</h1>
			<p class="keys-desc">
				{t('api.keys_desc', 'Manage your developer API keys. Each key identifies an application that can call the {site} API on your behalf.')}
			</p>
		</header>

		{#if newKey}
			<div class="new-key-banner" role="alert">
				<div class="new-key-head">
					<AlertTriangle size={16} />
					<span>
						{newKeyRotated
							? t('api.key_rotated_once', 'Your rotated API key — copy it now. It will not be shown again. The old key expires in 24 hours.')
							: t('api.key_created_once', 'Your new API key — copy it now. It will not be shown again.')}
					</span>
				</div>
				<div class="new-key-row">
					<code class="new-key-value">{newKey}</code>
					<button
						type="button"
						class="copy-btn"
						onclick={() => copyText(newKey, 'new')}
						aria-label={t('a11y.copy_code')}
					>
						{#if copiedId === 'new'}<Check size={14} />{:else}<Copy size={14} />{/if}
					</button>
					<button
						type="button"
						class="copy-btn"
						onclick={() => (newKey = '')}
						aria-label={t('a11y.dismiss', 'Dismiss')}
					>
						<X size={14} />
					</button>
				</div>
			</div>
		{/if}

		<div class="keys-toolbar">
			<button
				type="button"
				class="create-btn"
				onclick={openCreate}
				disabled={atLimit}
				title={atLimit ? t('api.max_keys_reached', 'Maximum keys reached') : ''}
			>
				<Plus size={14} />
				{t('api.new_key', 'New App')}
			</button>
			{#if atLimit}
				<span class="limit-note">
					{t('api.max_keys_reached', 'Maximum keys reached')} ({activeKeyCount}/{data.maxKeys})
				</span>
			{:else}
				<span class="limit-note">{activeKeyCount}/{data.maxKeys}</span>
			{/if}
		</div>

		{#if data.keys.length === 0}
			<div class="empty-state">
				<p>{t('api.no_keys_yet', 'You don’t have any API keys yet. Create one to start using the API.')}</p>
			</div>
		{:else}
			<table class="keys-table">
				<thead>
					<tr>
						<th>{t('api.app_name', 'App Name')}</th>
						<th>{t('api.key_prefix', 'Key')}</th>
						<th>{t('api.rate_limit_tier', 'Tier')}</th>
						<th>{t('api.scopes', 'Scopes')}</th>
						<th>{t('api.status', 'Status')}</th>
						<th>{t('api.last_used', 'Last Used')}</th>
						<th>{t('api.actions', 'Actions')}</th>
					</tr>
				</thead>
				<tbody>
					{#each data.keys as k (k.id)}
						<tr>
							<td class="app-name">{k.name}</td>
							<td>
								<code class="key-prefix">{k.key_prefix}…</code>
								{#if k.key_prefix}
									<button
										type="button"
										class="copy-btn small"
										onclick={() => copyText(k.key_prefix, k.id)}
										aria-label={t('a11y.copy_code')}
									>
										{#if copiedId === k.id}<Check size={12} />{:else}<Copy size={12} />{/if}
									</button>
								{/if}
							</td>
							<td><span class="tier-badge tier-{k.rate_limit_tier}">{k.rate_limit_tier}</span></td>
							<td class="scopes-cell">
								{#if k.scopes && k.scopes.length > 0}
									{#each k.scopes as s (s)}<code class="scope-chip">{s}</code>{/each}
								{:else}
									<span class="muted">—</span>
								{/if}
							</td>
							<td>
								<span class="status-badge status-{k.status}">{statusLabel(k.status)}</span>
								{#if expiryLabel(k)}
									<span class="expiry-note">{expiryLabel(k)}</span>
								{/if}
							</td>
							<td class="muted">
								{#if k.last_used_at}
									{new Date(k.last_used_at).toLocaleDateString()}
								{:else}
									{t('api.never_used', 'Never')}
								{/if}
							</td>
							<td>
								{#if k.status === 'active'}
									<div class="row-actions">
										<button
											type="button"
											class="row-btn"
											onclick={() => openDialog('rename', k)}
											title={t('api.rename', 'Rename')}
											aria-label={t('api.rename', 'Rename')}
										>
											<Pencil size={13} />
										</button>
										<button
											type="button"
											class="row-btn"
											onclick={() => openDialog('rotate', k)}
											title={t('api.rotate', 'Rotate')}
											aria-label={t('api.rotate', 'Rotate')}
										>
											<RefreshCw size={13} />
										</button>
										<button
											type="button"
											class="row-btn"
											onclick={() => openDialog('ips', k)}
											title={t('api.ip_allowlist', 'IP allowlist')}
											aria-label={t('api.ip_allowlist', 'IP allowlist')}
										>
											<Shield size={13} />
										</button>
										<button
											type="button"
											class="row-btn danger"
											onclick={() => openDialog('revoke', k)}
											title={t('api.revoke', 'Revoke')}
											aria-label={t('api.revoke', 'Revoke')}
										>
											<Trash2 size={13} />
										</button>
									</div>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}

		<!-- Scope explanations — sourced from GET /developer/scopes (§47 ROADMAP 06.05, Rule 16) -->
		<div class="scope-help">
			<h2>{t('api.scope_help_title', 'Available scopes')}</h2>
			<dl>
				{#each scopeOptions as scope (scope.value)}
					<dt><code>{scope.value}</code></dt>
					<dd>{scope.description}</dd>
				{/each}
			</dl>
		</div>

		<!-- Create dialog -->
		{#if showCreate}
			<div class="dialog-overlay" role="dialog" aria-modal="true" aria-label={t('api.new_key')}>
				<form class="dialog" method="POST" action="?/create" use:enhance={() => {
					creating = true;
					createError = '';
					return async ({ update }) => {
						creating = false;
						// update() assigns `form` + invalidates — the $effect on
						// `form` then shows the secret / error and refreshes the list.
						await update();
					};
				}}>
					<h2>{t('api.create_key', 'Create API Key')}</h2>
					{#if createError}
						<p class="form-error">{createError}</p>
					{/if}
					<label>
						<span>{t('api.app_name', 'App Name')}</span>
						<input name="name" type="text" bind:value={keyName} placeholder="My App" required />
					</label>
					<label>
						<span>{t('api.rate_limit_tier', 'Rate limit tier')}</span>
						<AutocompleteSelect
							name="rate_limit_tier"
							bind:value={rateLimitTier}
							options={(['basic', 'standard', 'premium', 'unlimited'] as const).map((tier) => ({
								value: tier,
								label: t(`api.tier.${tier}`, tier)
							}))}
						/>
					</label>
					<div class="scope-field">
						<span class="scope-field-label">{t('api.scopes', 'Scopes')}</span>
						<Autocomplete
							placeholder={t('api.add_scope', 'Add a scope…')}
							ariaLabel={t('api.add_scope', 'Add a scope')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={scopeFetcher}
							initialFetcher={() => scopeFetcher('')}
							selectedId={null}
							onSelect={addScope}
						/>
						{#if selectedScopes.length > 0}
							<div class="scope-chips">
								{#each selectedScopes as s (s)}
									<span class="scope-chip">
										<code>{s}</code>
										<button
											type="button"
											class="scope-chip-x"
											onclick={() => removeScope(s)}
											aria-label={t('common.remove', 'Remove')}
										>
											<X size={11} />
										</button>
									</span>
								{/each}
							</div>
						{/if}
						<input type="hidden" name="scopes" value={selectedScopes.join(',')} />
					</div>
					<div class="dialog-actions">
						<button type="button" class="cancel-btn" onclick={() => (showCreate = false)}>
							{t('api.cancel', 'Cancel')}
						</button>
						<button type="submit" class="submit-btn" disabled={creating || !keyName.trim()}>
							{creating ? t('api.creating', 'Creating…') : t('api.create', 'Create')}
						</button>
					</div>
				</form>
			</div>
		{/if}

		<!-- Rename dialog -->
		{#if dialog === 'rename' && dialogKey}
			<div class="dialog-overlay" role="dialog" aria-modal="true" aria-label={t('api.rename', 'Rename')}>
				<form class="dialog" method="POST" action="?/rename" use:enhance={() => {
					dialogWorking = true;
					dialogError = '';
					return async ({ update }) => { dialogWorking = false; await update(); };
				}}>
					<h2>{t('api.rename_key', 'Rename API Key')}</h2>
					{#if dialogError}
						<p class="form-error">{dialogError}</p>
					{/if}
					<label>
						<span>{t('api.app_name', 'App Name')}</span>
						<input name="name" type="text" bind:value={renameName} required />
					</label>
					<input type="hidden" name="id" value={dialogKey.id} />
					<div class="dialog-actions">
						<button type="button" class="cancel-btn" onclick={closeDialog}>
							{t('api.cancel', 'Cancel')}
						</button>
						<button type="submit" class="submit-btn" disabled={dialogWorking || !renameName.trim()}>
							{t('api.save', 'Save')}
						</button>
					</div>
				</form>
			</div>
		{/if}

		<!-- Rotate confirmation dialog -->
		{#if dialog === 'rotate' && dialogKey}
			<div class="dialog-overlay" role="dialog" aria-modal="true" aria-label={t('api.rotate', 'Rotate')}>
				<form class="dialog confirm" method="POST" action="?/rotate" use:enhance={() => {
					dialogWorking = true;
					dialogError = '';
					return async ({ update }) => { dialogWorking = false; await update(); };
				}}>
					<h2>{t('api.rotate_key', 'Rotate API Key')}</h2>
					{#if dialogError}
						<p class="form-error">{dialogError}</p>
					{/if}
					<p class="confirm-text">
						{t('api.rotate_warning', 'A new secret will be generated for')}
						<strong>{dialogKey.name}</strong>.
						{t('api.rotate_grace', 'The old key keeps working for 24 hours, then expires permanently.')}
					</p>
					<input type="hidden" name="id" value={dialogKey.id} />
					<div class="dialog-actions">
						<button type="button" class="cancel-btn" onclick={closeDialog}>
							{t('api.cancel', 'Cancel')}
						</button>
						<button type="submit" class="submit-btn" disabled={dialogWorking}>
							<RefreshCw size={14} />
							{t('api.rotate', 'Rotate')}
						</button>
					</div>
				</form>
			</div>
		{/if}

		<!-- IP allowlist dialog -->
		{#if dialog === 'ips' && dialogKey}
			{@const entries = data.allowlists[dialogKey.id] ?? []}
			<div class="dialog-overlay" role="dialog" aria-modal="true" aria-label={t('api.ip_allowlist', 'IP allowlist')}>
				<div class="dialog">
					<h2>{t('api.ip_allowlist_title', 'IP allowlist')} — {dialogKey.name}</h2>
					{#if dialogError}
						<p class="form-error">{dialogError}</p>
					{/if}
					<p class="confirm-text">
						{t('api.ip_allowlist_help', 'When entries exist, only requests from these IPs/CIDRs can use this key.')}
					</p>
					{#if entries.length === 0}
						<p class="muted ip-empty">{t('api.ip_allowlist_empty', 'No restrictions — the key works from any IP.')}</p>
					{:else}
						<ul class="ip-list">
							{#each entries as entry (entry.id)}
								<li class="ip-entry">
									<code>{entry.cidr}</code>
									{#if entry.description}<span class="muted ip-desc">{entry.description}</span>{/if}
									<form method="POST" action="?/removeIp" use:enhance={() => {
										return async ({ update }) => { await update(); };
									}}>
										<input type="hidden" name="id" value={dialogKey.id} />
										<input type="hidden" name="ip_id" value={entry.id} />
										<button type="submit" class="row-btn danger" aria-label={t('common.remove', 'Remove')}>
											<Trash2 size={12} />
										</button>
									</form>
								</li>
							{/each}
						</ul>
					{/if}
					<form class="ip-add" method="POST" action="?/addIp" use:enhance={() => {
						dialogWorking = true;
						dialogError = '';
						return async ({ update }) => { dialogWorking = false; await update(); };
					}}>
						<input type="hidden" name="id" value={dialogKey.id} />
						<label>
							<span>{t('api.ip_cidr', 'IP or CIDR')}</span>
							<input name="cidr" type="text" bind:value={ipCidr} placeholder="203.0.113.0/24" required />
						</label>
						<label>
							<span>{t('api.ip_description', 'Description')} <span class="muted">({t('common.optional', 'optional')})</span></span>
							<input name="description" type="text" bind:value={ipDesc} placeholder={t('api.ip_desc_placeholder', 'Office egress')} />
						</label>
						<div class="dialog-actions">
							<button type="button" class="cancel-btn" onclick={closeDialog}>
								{t('api.close', 'Close')}
							</button>
							<button type="submit" class="submit-btn" disabled={dialogWorking || !ipCidr.trim()}>
								{t('api.ip_add', 'Add entry')}
							</button>
						</div>
					</form>
				</div>
			</div>
		{/if}

		<!-- Revoke confirmation dialog -->
		{#if dialog === 'revoke' && dialogKey}
			<div class="dialog-overlay" role="dialog" aria-modal="true" aria-label={t('api.revoke')}>
				<form class="dialog confirm" method="POST" action="?/revoke" use:enhance={() => {
					dialogWorking = true;
					dialogError = '';
					return async ({ update }) => { dialogWorking = false; await update(); };
				}}>
					<h2>{t('api.revoke_confirm', 'Revoke API Key')}</h2>
					{#if dialogError}
						<p class="form-error">{dialogError}</p>
					{/if}
					<p class="confirm-text">
						{t('api.revoke_warning', 'Are you sure you want to revoke the key for')}
						<strong>{dialogKey.name}</strong>?
						{t('api.revoke_irreversible', 'This cannot be undone. All requests using this key will immediately fail.')}
					</p>
					<input type="hidden" name="id" value={dialogKey.id} />
					<div class="dialog-actions">
						<button type="button" class="cancel-btn" onclick={closeDialog}>
							{t('api.cancel', 'Cancel')}
						</button>
						<button type="submit" class="revoke-confirm-btn" disabled={dialogWorking}>
							<Trash2 size={14} />
							{t('api.revoke', 'Revoke')}
						</button>
					</div>
				</form>
			</div>
		{/if}
	</section>
</AppShell>

<style>
	.keys-page {
		max-width: 1040px;
		margin: 0 auto;
		padding: 2.5rem 1.5rem 4rem;
	}
	.keys-header { margin-bottom: 2rem; }
	.back-link {
		font-size: 0.8125rem;
		color: var(--color-muted);
		text-decoration: none;
	}
	.back-link:hover { color: var(--color-primary); }
	.keys-header h1 {
		font-family: var(--font-heading, var(--font-sans));
		font-size: 2rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0.5rem 0 0;
		letter-spacing: -0.02em;
	}
	.keys-desc {
		font-size: 0.9375rem;
		color: var(--color-text-subtle);
		margin: 0.5rem 0 0;
		line-height: 1.5;
	}

	.new-key-banner {
		border: 1px solid var(--method-patch);
		background: var(--method-patch-bg);
		border-radius: var(--radius-card);
		padding: 1rem 1.25rem;
		margin-bottom: 1.5rem;
	}
	.new-key-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--method-patch);
		margin-bottom: 0.75rem;
	}
	.new-key-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.new-key-value {
		flex: 1;
		font-family: var(--font-mono);
		font-size: 0.875rem;
		background: var(--color-bg);
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius);
		color: var(--color-text);
		word-break: break-all;
	}

	.keys-toolbar {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-bottom: 1.5rem;
	}
	.create-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.5rem 1rem;
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		border: none;
		border-radius: var(--radius);
		font-size: 0.875rem;
		font-weight: 600;
		cursor: pointer;
	}
	.create-btn:hover { background: var(--color-primary-hover); }
	.create-btn:disabled { opacity: 0.5; cursor: not-allowed; }
	.limit-note {
		font-size: 0.75rem;
		color: var(--color-muted);
	}

	.empty-state {
		padding: 3rem 2rem;
		text-align: center;
		border: 1px dashed var(--color-border);
		border-radius: var(--radius-card);
		color: var(--color-muted);
		font-size: 0.9375rem;
	}

	.keys-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.875rem;
	}
	.keys-table th {
		text-align: start;
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		padding: 0.5rem 0.75rem;
		border-bottom: 1px solid var(--color-border);
	}
	.keys-table td {
		padding: 0.625rem 0.75rem;
		border-bottom: 1px solid var(--color-border);
		color: var(--color-text);
		vertical-align: middle;
	}
	.app-name { font-weight: 600; }
	.key-prefix {
		font-family: var(--font-mono);
		font-size: 0.8125rem;
		background: var(--color-surface-subtle);
		padding: 0.125rem 0.375rem;
		border-radius: 4px;
	}
	.tier-badge {
		font-size: 0.6875rem;
		text-transform: uppercase;
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		font-weight: 600;
	}
	.tier-basic { color: var(--method-get); background: var(--method-get-bg); }
	.tier-standard { color: var(--method-put); background: var(--method-put-bg); }
	.tier-premium { color: var(--method-patch); background: var(--method-patch-bg); }
	.tier-unlimited { color: var(--method-delete); background: var(--method-delete-bg); }
	.scopes-cell {
		max-width: 220px;
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem;
	}
	.scope-chip {
		font-family: var(--font-mono);
		font-size: 0.6875rem;
		color: var(--color-text-subtle);
		background: var(--color-surface-subtle);
		border: 1px solid var(--color-border);
		border-radius: 4px;
		padding: 0.0625rem 0.375rem;
		white-space: nowrap;
	}
	.status-badge {
		display: inline-block;
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		font-weight: 600;
	}
	.status-active { color: var(--method-get); background: var(--method-get-bg); }
	.status-suspended { color: var(--method-patch); background: var(--method-patch-bg); }
	.status-revoked { color: var(--method-delete); background: var(--method-delete-bg); }
	.expiry-note {
		display: block;
		font-size: 0.6875rem;
		color: var(--color-muted);
		margin-top: 0.25rem;
	}
	.muted { color: var(--color-muted); }

	.row-actions {
		display: flex;
		gap: 0.25rem;
	}
	.row-btn {
		display: inline-grid;
		place-items: center;
		height: 1.75rem;
		width: 1.75rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-bg);
		color: var(--color-text-subtle);
		cursor: pointer;
	}
	.row-btn:hover { color: var(--color-primary); border-color: var(--color-primary); }
	.row-btn.danger { color: var(--method-delete); }
	.row-btn.danger:hover { border-color: var(--method-delete); }

	.copy-btn {
		display: inline-grid;
		place-items: center;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-bg);
		color: var(--color-muted);
		cursor: pointer;
		padding: 0.25rem;
	}
	.copy-btn.small { padding: 0.125rem; }
	.copy-btn:hover { color: var(--color-primary); border-color: var(--color-primary); }

	.scope-help {
		margin-top: 2.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid var(--color-border);
	}
	.scope-help h2 {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		margin: 0 0 0.75rem;
	}
	.scope-help dl {
		display: grid;
		grid-template-columns: 200px 1fr;
		gap: 0.5rem 1rem;
		font-size: 0.8125rem;
	}
	.scope-help dt code {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--color-primary);
	}
	.scope-help dd {
		margin: 0;
		color: var(--color-text-subtle);
	}

	.dialog-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 100;
		padding: 1rem;
	}
	.dialog {
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		padding: 1.5rem;
		max-width: 440px;
		width: 100%;
		display: flex;
		flex-direction: column;
		gap: 1rem;
		max-height: calc(100vh - 4rem);
		overflow-y: auto;
	}
	.dialog.confirm { max-width: 400px; }
	.dialog h2 {
		font-size: 1.125rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0;
	}
	.confirm-text {
		font-size: 0.875rem;
		color: var(--color-text-subtle);
		line-height: 1.5;
		margin: 0;
	}
	.form-error {
		font-size: 0.8125rem;
		color: var(--method-delete);
		margin: 0;
	}
	.dialog label {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		font-size: 0.8125rem;
		color: var(--color-text);
	}
	.dialog input {
		width: 100%;
		padding: 0.5rem 0.625rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-bg);
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
	}
	.dialog input:focus { border-color: var(--color-primary); }
	.dialog-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
	}
	.cancel-btn {
		padding: 0.5rem 1rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-bg);
		color: var(--color-text);
		font-size: 0.875rem;
		cursor: pointer;
	}
	.submit-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.5rem 1rem;
		border: none;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		font-size: 0.875rem;
		font-weight: 600;
		cursor: pointer;
	}
	.submit-btn:disabled { opacity: 0.5; cursor: not-allowed; }
	.revoke-confirm-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0.5rem 1rem;
		border: none;
		border-radius: var(--radius);
		background: var(--method-delete);
		color: white;
		font-size: 0.875rem;
		font-weight: 600;
		cursor: pointer;
	}
	.revoke-confirm-btn:disabled { opacity: 0.5; cursor: not-allowed; }

	.scope-field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.8125rem;
	}
	.scope-field-label { color: var(--color-text); }
	.scope-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 0.375rem;
	}
	.scope-chip {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0.125rem 0.375rem;
		border: 1px solid var(--color-border);
		border-radius: 9999px;
		background: var(--color-surface-subtle);
	}
	.scope-chip code {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--color-text);
	}
	.scope-chip-x {
		display: inline-grid;
		place-items: center;
		width: 1rem;
		height: 1rem;
		border: none;
		border-radius: 50%;
		background: transparent;
		color: var(--color-muted);
		cursor: pointer;
		padding: 0;
	}
	.scope-chip-x:hover { color: var(--method-delete); }

	.ip-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}
	.ip-entry {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.375rem 0.5rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		font-size: 0.8125rem;
	}
	.ip-entry code {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--color-text);
	}
	.ip-entry form { margin-inline-start: auto; }
	.ip-desc { font-size: 0.75rem; }
	.ip-empty { font-size: 0.8125rem; }
	.ip-add {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
</style>
