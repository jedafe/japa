<script lang="ts">
	import { untrack } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { t } from '$lib/i18n';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import { Plus, Trash2, Copy, Check, AlertTriangle } from 'lucide-svelte';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	// Create-key dialog state
	let showCreate = $state(false);
	let appName = $state('');
	let environment = $state<'live' | 'test'>('live');
	let scopesInput = $state('');
	let creating = $state(false);
	let createError = $state('');

	// Newly created key — shown once (Ref: §47 ROADMAP 06.02)
	let newKey = $state('');
	let copiedNew = $state(false);

	// Revoke confirmation
	let revokeId = $state<string | null>(null);
	let revokeName = $state('');

	// Copied prefix indicator
	let copiedId = $state<string | null>(null);

	let atLimit = $derived(data.keys.length >= data.maxKeys);

	// Fallback scope descriptions for the help section (meta-tooling, not platform content — Ref: §47 ROADMAP 06.05)
	// Unknown scopes still display — they just use the default description below.
	const scopeDescriptions: Record<string, string> = {
		'api.read.spaces': 'Read spaces, spaces metadata, and space membership',
		'api.read.content': 'Read published content (posts, pages, threads)',
		'api.write.content': 'Create and update content on behalf of the user',
		'api.read.users': 'Read user profiles and public settings',
		'api.read.search': 'Search the platform via the search API'
	};

	// All known scopes (hardcoded + any from the user's existing keys)
	let allScopes = $derived.by(() => {
		const scopes = new Set(Object.keys(scopeDescriptions));
		for (const k of data.keys) {
			if (k.scopes) {
				for (const s of k.scopes) scopes.add(s);
			}
		}
		return [...scopes].sort();
	});

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
		appName = '';
		environment = 'live';
		scopesInput = '';
		createError = '';
		newKey = '';
	}

	function confirmRevoke(id: string, name: string) {
		revokeId = id;
		revokeName = name;
	}

	// Handle form action results
	$effect(() => {
		if (form?.created) {
			newKey = form.created.key;
			copiedNew = false;
			showCreate = false;
			invalidateAll();
		}
		if (form?.revoked) {
			revokeId = null;
			invalidateAll();
		}
		if (form?.error) {
			createError = form.error;
		}
	});
</script>

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
				{t('api.keys_desc', 'Manage your developer API keys. Each key identifies an application that can call the JapaConnect API on your behalf.')}
			</p>
		</header>

		{#if newKey}
			<div class="new-key-banner" role="alert">
				<div class="new-key-head">
					<AlertTriangle size={16} />
					<span>{t('api.key_created_once', 'Your new API key — copy it now. It will not be shown again.')}</span>
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
					{t('api.max_keys_reached', 'Maximum keys reached')} ({data.keys.length}/{data.maxKeys})
				</span>
			{:else}
				<span class="limit-note">{data.keys.length}/{data.maxKeys}</span>
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
						<th>{t('api.environment', 'Env')}</th>
						<th>{t('api.scopes', 'Scopes')}</th>
						<th>{t('api.last_used', 'Last Used')}</th>
						<th>{t('api.actions', 'Actions')}</th>
					</tr>
				</thead>
				<tbody>
					{#each data.keys as k (k.id)}
						<tr>
							<td class="app-name">{k.app_name}</td>
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
							<td><span class="env-badge env-{k.environment}">{k.environment}</span></td>
							<td class="scopes-cell">
								{#if k.scopes && k.scopes.length > 0}
									{#each k.scopes as s}{s} {/each}
								{:else}
									<span class="muted">—</span>
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
									<button
										type="button"
										class="revoke-btn"
										onclick={() => confirmRevoke(k.id, k.app_name)}
									>
										<Trash2 size={14} />
										{t('api.revoke', 'Revoke')}
									</button>
								{:else}
									<span class="revoked-badge">{t('api.revoked', 'revoked')}</span>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}

		<!-- Scope explanations (Ref: §47 ROADMAP 06.05) -->
		<div class="scope-help">
			<h2>{t('api.scope_help_title', 'Available scopes')}</h2>
			<dl>
				{#each allScopes as scope}
					<dt><code>{scope}</code></dt>
					<dd>{scopeDescriptions[scope] ?? 'API scope — see documentation for details'}</dd>
				{/each}
			</dl>
		</div>

		<!-- Create dialog -->
		{#if showCreate}
			<div class="dialog-overlay" role="dialog" aria-modal="true" aria-label={t('api.new_key')}>
				<form class="dialog" method="POST" action="?/create" use:enhance={() => {
					creating = true;
					createError = '';
					return async ({ result }) => {
						creating = false;
						if (result.type === 'success') {
							// form effect handles the rest
						}
					};
				}}>
					<h2>{t('api.create_key', 'Create API Key')}</h2>
					{#if createError}
						<p class="form-error">{createError}</p>
					{/if}
					<label>
						<span>{t('api.app_name', 'App Name')}</span>
						<input name="app_name" type="text" bind:value={appName} placeholder="My App" required />
					</label>
					<label>
						<span>{t('api.environment', 'Environment')}</span>
						<select name="environment" bind:value={environment}>
							<option value="live">live</option>
							<option value="test">test</option>
						</select>
					</label>
					<label>
						<span>{t('api.scopes', 'Scopes')} <span class="muted">(comma-separated)</span></span>
						<input name="scopes" type="text" bind:value={scopesInput} placeholder="api.read.spaces, api.read.content" />
					</label>
					<div class="dialog-actions">
						<button type="button" class="cancel-btn" onclick={() => (showCreate = false)}>
							{t('api.cancel', 'Cancel')}
						</button>
						<button type="submit" class="submit-btn" disabled={creating || !appName.trim()}>
							{creating ? t('api.creating', 'Creating…') : t('api.create', 'Create')}
						</button>
					</div>
				</form>
			</div>
		{/if}

		<!-- Revoke confirmation dialog -->
		{#if revokeId}
			<div class="dialog-overlay" role="dialog" aria-modal="true" aria-label={t('api.revoke')}>
				<form class="dialog confirm" method="POST" action="?/revoke" use:enhance>
					<h2>{t('api.revoke_confirm', 'Revoke API Key')}</h2>
					<p class="confirm-text">
						{t('api.revoke_warning', 'Are you sure you want to revoke the key for')}
						<strong>{revokeName}</strong>?
						{t('api.revoke_irreversible', 'This cannot be undone. All requests using this key will immediately fail.')}
					</p>
					<input type="hidden" name="id" value={revokeId} />
					<div class="dialog-actions">
						<button type="button" class="cancel-btn" onclick={() => (revokeId = null)}>
							{t('api.cancel', 'Cancel')}
						</button>
						<button type="submit" class="revoke-confirm-btn">
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
		max-width: 900px;
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
		font-family: var(--font-sans);
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
		text-align: left;
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
	.env-badge {
		font-size: 0.6875rem;
		text-transform: uppercase;
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		font-weight: 600;
	}
	.env-live { color: var(--method-get); background: var(--method-get-bg); }
	.env-test { color: var(--method-patch); background: var(--method-patch-bg); }
	.scopes-cell {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--color-text-subtle);
	}
	.muted { color: var(--color-muted); }
	.revoke-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0.25rem 0.5rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-bg);
		color: var(--method-delete);
		font-size: 0.75rem;
		cursor: pointer;
	}
	.revoke-btn:hover { border-color: var(--method-delete); }
	.revoked-badge {
		font-size: 0.6875rem;
		text-transform: uppercase;
		color: var(--color-muted);
	}

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
	.dialog input, .dialog select {
		width: 100%;
		padding: 0.5rem 0.625rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-bg);
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
	}
	.dialog input:focus, .dialog select:focus { border-color: var(--color-primary); }
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
</style>
