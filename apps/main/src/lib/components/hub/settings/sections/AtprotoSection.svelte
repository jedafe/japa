<script lang="ts">
	/**
	 * AT Protocol (Bluesky) identity — Section 127 / ADR-139.
	 *
	 * Member-facing surface for the native atproto account: opt-in/out
	 * (POST/DELETE /me/atproto), repo CAR export (GET /me/atproto/export),
	 * app-password management, and OAuth session review — mirroring the
	 * fediverse section next to it (Rule 16: everything from the API).
	 *
	 * Ref: japaconnect-engineering/sections/127-atproto-native.md
	 *      japaconnect-engineering/adr/ADR-139-atproto-native.md
	 */
	import { Modal } from '#lib/components/ui/index.js';
	import { invalidateAll } from '$app/navigation';
	import { api } from '#lib/api/client.js';
	import { t } from '#lib/i18n/index.js';
	import { KeyRound, Download, Trash2, Copy } from 'lucide-svelte';

	type AtprotoStatus = {
		enabled?: boolean;
		status?: string;
		did?: string;
		handle?: string;
		created_at?: string;
	};

	type AppPassword = {
		id: string;
		name: string;
		created_at?: string;
		last_used_at?: string | null;
	};

	type OAuthSession = {
		client_id: string;
		scope?: string;
		first_granted_at?: string;
		last_granted_at?: string;
	};

	let { data }: { data: unknown } = $props();
	const sectionData = $derived(
		data as {
			status: AtprotoStatus | null;
			username?: string;
			appPasswords: AppPassword[];
			oauthSessions: OAuthSession[];
		} | null ?? null
	);
	const acct = $derived(sectionData?.status ?? null);
	const appPasswords = $derived(sectionData?.appPasswords ?? []);
	const oauthSessions = $derived(sectionData?.oauthSessions ?? []);
	const enabled = $derived(acct?.enabled === true);

	// §127: the member's username becomes the handle's local part, which
	// must be DNS-legal — lowercase letters, digits, hyphens, 2–63 chars.
	// The backend's validHandle enforces the same regex plus a reserved
	// list; this preview only mirrors the charset rule (the 400 path below
	// still covers reserved names).
	const username = $derived(sectionData?.username ?? '');
	const handleLocalOk = $derived(/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(username.toLowerCase()));

	let busy = $state(false);
	let exportBusy = $state(false);
	let errorMsg = $state('');
	let notice = $state('');

	let newPwName = $state('');
	/** The plaintext secret — shown once after creation, then discarded. */
	let createdSecret = $state<{ name: string; password: string } | null>(null);
	let copied = $state(false);

	let disableOpen = $state(false);
	let revokePwOpen = $state(false);
	let revokeOAuthOpen = $state(false);
	let revokePw = $state<AppPassword | null>(null);
	let revokeOAuth = $state<OAuthSession | null>(null);

	async function run(fn: () => Promise<boolean>) {
		if (busy) return;
		busy = true;
		errorMsg = '';
		notice = '';
		try {
			if (await fn()) await invalidateAll();
		} finally {
			busy = false;
		}
	}

	function setError(res: { error?: { code?: string; message?: string } | null }) {
		if (res.error?.code === 'invalid_handle') {
			errorMsg = t('settings.atproto.invalid_handle_msg', "Your username can't be used as a Bluesky handle — it must be 2–63 chars of lowercase letters, numbers, or hyphens. Change your username in Account settings to enable.");
		} else if (res.error?.code === 'handle_taken') {
			errorMsg = t('settings.atproto.handle_taken_msg', 'That handle is already claimed by another member.');
		} else {
			errorMsg = res.error?.message ?? t('settings.atproto.action_failed', 'The request failed. Try again.');
		}
		return false;
	}

	function handleEnable() {
		return run(async () => {
			const res = await api.post<AtprotoStatus>('/me/atproto');
			if (res.error || !res.data) return setError(res);
			notice = t('settings.atproto.enabled_msg', 'Your Bluesky identity is live.');
			return true;
		});
	}

	function handleDisable() {
		return run(async () => {
			const res = await api.delete<AtprotoStatus>('/me/atproto');
			if (res.error) return setError(res);
			disableOpen = false;
			notice = t('settings.atproto.disabled_msg', 'Your Bluesky identity is disabled. Your DID and handle stay reserved.');
			return true;
		});
	}

	async function handleExport() {
		if (exportBusy) return;
		exportBusy = true;
		errorMsg = '';
		try {
			const res = await api.download('/me/atproto/export');
			if (!res.blob) {
				errorMsg = t('settings.atproto.export_failed', 'Export failed — try again.');
				return;
			}
			const url = URL.createObjectURL(res.blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = res.filename ?? 'repo.car';
			document.body.appendChild(a);
			a.click();
			a.remove();
			URL.revokeObjectURL(url);
		} finally {
			exportBusy = false;
		}
	}

	function handleCreatePassword() {
		const name = newPwName.trim();
		if (!name) return;
		return run(async () => {
			const res = await api.post<{ id: string; name: string; password: string }>(
				'/me/atproto/app-passwords',
				{ name }
			);
			if (res.error || !res.data) return setError(res);
			createdSecret = { name: res.data.name, password: res.data.password };
			newPwName = '';
			return true;
		});
	}

	function handleRevokePassword() {
		const target = revokePw;
		if (!target) return;
		return run(async () => {
			const res = await api.delete(`/me/atproto/app-passwords/${target.id}`);
			if (res.error) return setError(res);
			revokePwOpen = false;
			return true;
		});
	}

	function handleRevokeOAuth() {
		const target = revokeOAuth;
		if (!target) return;
		return run(async () => {
			const res = await api.delete(
				`/me/atproto/oauth-sessions?client_id=${encodeURIComponent(target.client_id)}`
			);
			if (res.error) return setError(res);
			revokeOAuthOpen = false;
			return true;
		});
	}

	async function copySecret() {
		if (!createdSecret) return;
		try {
			await navigator.clipboard.writeText(createdSecret.password);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			/* clipboard unavailable — the user can still select the text */
		}
	}

	function fmtDate(v?: string | null): string {
		if (!v) return '—';
		const d = new Date(v);
		return Number.isNaN(d.getTime()) ? '—' : d.toLocaleDateString();
	}
</script>

<div class="flex flex-col gap-6">
	<section class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="text-base font-semibold text-[var(--color-text)]">
			{t('settings.atproto.title', 'Bluesky (AT Protocol)')}
		</h2>
		<p class="mt-1 text-sm text-[var(--color-muted)]">
			{t('settings.atproto.desc', 'Mirror your JapaConnect presence on the AT Protocol network — your posts become Bluesky-compatible records under your own DID.')}
		</p>

		{#if errorMsg}
			<p role="alert" class="mt-3 rounded-[var(--radius)] border border-[var(--color-danger,#dc2626)]/40 bg-[var(--color-danger,#dc2626)]/10 px-3 py-2 text-sm text-[var(--color-danger,#dc2626)]">{errorMsg}</p>
		{/if}
		{#if notice}
			<p role="status" class="mt-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text)]">{notice}</p>
		{/if}

		{#if !enabled}
			<div class="mt-4">
				{#if username && !handleLocalOk}
					<p class="text-sm text-[var(--color-muted)]">
						{t('settings.atproto.handle_invalid_hint', 'Your username "{username}" can\'t be a Bluesky handle — handles allow only lowercase letters, numbers, and hyphens (2–63 chars). Update your username in Account settings to enable.', { username })}
					</p>
				{:else}
					<button
						type="button"
						disabled={busy}
						onclick={handleEnable}
						class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
					>
						{busy ? t('common.submitting', 'Submitting…') : t('settings.atproto.enable', 'Enable Bluesky identity')}
					</button>
					<p class="mt-2 text-xs text-[var(--color-muted)]">
						{t('settings.atproto.enable_hint', 'Creates a DID for your account and starts publishing your public posts as AT Protocol records.')}
					</p>
				{/if}
			</div>
		{:else}
			<dl class="mt-4 flex flex-col gap-2 text-sm">
				<div class="flex flex-wrap items-baseline gap-x-2">
					<dt class="font-medium text-[var(--color-muted)]">{t('settings.atproto.handle', 'Handle')}</dt>
					<dd class="font-mono text-[var(--color-text)]">{acct?.handle ?? '—'}</dd>
				</div>
				<div class="flex flex-wrap items-baseline gap-x-2">
					<dt class="font-medium text-[var(--color-muted)]">{t('settings.atproto.did', 'DID')}</dt>
					<dd class="font-mono break-all text-xs text-[var(--color-text)]">{acct?.did ?? '—'}</dd>
				</div>
				<div class="flex flex-wrap items-baseline gap-x-2">
					<dt class="font-medium text-[var(--color-muted)]">{t('settings.atproto.status', 'Status')}</dt>
					<dd class="text-[var(--color-text)]">{acct?.status ?? '—'}</dd>
				</div>
			</dl>

			<div class="mt-4 flex flex-wrap gap-2">
				<button
					type="button"
					disabled={exportBusy}
					onclick={handleExport}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
				>
					<Download size={14} aria-hidden="true" />
					{exportBusy ? t('common.submitting', 'Submitting…') : t('settings.atproto.export_repo', 'Export repository (.car)')}
				</button>
				<button
					type="button"
					disabled={busy}
					onclick={() => (disableOpen = true)}
					class="rounded-[var(--radius)] border border-[var(--color-danger,#dc2626)]/50 px-3 py-1.5 text-sm text-[var(--color-danger,#dc2626)] hover:bg-[var(--color-danger,#dc2626)]/10 disabled:opacity-50"
				>
					{t('settings.atproto.disable', 'Disable')}
				</button>
			</div>
		{/if}
	</section>

	{#if enabled}
		<!-- App passwords -->
		<section class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h3 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
				<KeyRound size={15} aria-hidden="true" />
				{t('settings.atproto.app_passwords', 'App passwords')}
			</h3>
			<p class="mt-1 text-xs text-[var(--color-muted)]">
				{t('settings.atproto.app_passwords_desc', 'Scoped passwords for third-party atproto clients. A secret is shown once at creation — store it somewhere safe.')}
			</p>

			<form class="mt-3 flex flex-wrap gap-2" onsubmit={(e) => { e.preventDefault(); handleCreatePassword(); }}>
				<input
					type="text"
					bind:value={newPwName}
					maxlength="64"
					placeholder={t('settings.atproto.password_name_placeholder', 'Password name (e.g. my-client)')}
					aria-label={t('settings.atproto.password_name_placeholder', 'Password name (e.g. my-client)')}
					class="min-w-52 flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-1.5 text-sm text-[var(--color-text)]"
				/>
				<button
					type="submit"
					disabled={busy || newPwName.trim() === ''}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
				>
					{t('settings.atproto.create_password', 'Create password')}
				</button>
			</form>

			{#if createdSecret}
				<div class="mt-3 rounded-[var(--radius)] border border-[var(--color-primary)]/40 bg-[var(--color-primary)]/5 p-3" role="alert">
					<p class="text-xs font-medium text-[var(--color-text)]">
						{t('settings.atproto.secret_once', 'Copy this secret now — it will not be shown again.')}
					</p>
					<div class="mt-2 flex items-center gap-2">
						<code class="flex-1 break-all rounded bg-[var(--color-bg)] px-2 py-1 font-mono text-xs text-[var(--color-text)]">{createdSecret.password}</code>
						<button
							type="button"
							onclick={copySecret}
							class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
						>
							<Copy size={12} aria-hidden="true" />
							{copied ? t('common.copied', 'Copied') : t('common.copy', 'Copy')}
						</button>
					</div>
					<button
						type="button"
						onclick={() => (createdSecret = null)}
						class="mt-2 text-xs text-[var(--color-muted)] underline hover:text-[var(--color-text)]"
					>
						{t('settings.atproto.secret_dismiss', 'I saved it — dismiss')}
					</button>
				</div>
			{/if}

			{#if appPasswords.length === 0}
				<p class="mt-3 text-sm text-[var(--color-muted)]">{t('settings.atproto.no_passwords', 'No app passwords yet.')}</p>
			{:else}
				<ul class="mt-3 divide-y divide-[var(--color-border)]">
					{#each appPasswords as pw (pw.id)}
						<li class="flex items-center justify-between gap-3 py-2">
							<div class="min-w-0">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">{pw.name}</p>
								<p class="text-xs text-[var(--color-muted)]">
									{t('settings.atproto.created', 'Created')} {fmtDate(pw.created_at)}{#if pw.last_used_at} · {t('settings.atproto.last_used', 'Last used')} {fmtDate(pw.last_used_at)}{/if}
								</p>
							</div>
							<button
								type="button"
								disabled={busy}
								onclick={() => { revokePw = pw; revokePwOpen = true; }}
								class="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] px-2 py-1 text-xs text-[var(--color-danger,#dc2626)] hover:bg-[var(--color-danger,#dc2626)]/10 disabled:opacity-50"
								aria-label={t('settings.atproto.revoke', 'Revoke')}
							>
								<Trash2 size={13} aria-hidden="true" />
								{t('settings.atproto.revoke', 'Revoke')}
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<!-- OAuth sessions -->
		<section class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h3 class="text-sm font-semibold text-[var(--color-text)]">
				{t('settings.atproto.oauth_sessions', 'Connected apps')}
			</h3>
			<p class="mt-1 text-xs text-[var(--color-muted)]">
				{t('settings.atproto.oauth_sessions_desc', 'Third-party atproto clients you authorized with OAuth. Revoking drops every token for that client.')}
			</p>

			{#if oauthSessions.length === 0}
				<p class="mt-3 text-sm text-[var(--color-muted)]">{t('settings.atproto.no_sessions', 'No connected apps.')}</p>
			{:else}
				<ul class="mt-3 divide-y divide-[var(--color-border)]">
					{#each oauthSessions as s (s.client_id)}
						<li class="flex items-center justify-between gap-3 py-2">
							<div class="min-w-0">
								<p class="truncate font-mono text-xs text-[var(--color-text)]">{s.client_id}</p>
								<p class="text-xs text-[var(--color-muted)]">
									{t('settings.atproto.granted', 'Granted')} {fmtDate(s.first_granted_at)}{#if s.last_granted_at} · {t('settings.atproto.last_granted', 'Latest')} {fmtDate(s.last_granted_at)}{/if}
								</p>
								{#if s.scope}
									<p class="truncate text-xs text-[var(--color-muted)]">{s.scope}</p>
								{/if}
							</div>
							<button
								type="button"
								disabled={busy}
								onclick={() => { revokeOAuth = s; revokeOAuthOpen = true; }}
								class="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] px-2 py-1 text-xs text-[var(--color-danger,#dc2626)] hover:bg-[var(--color-danger,#dc2626)]/10 disabled:opacity-50"
							>
								<Trash2 size={13} aria-hidden="true" />
								{t('settings.atproto.revoke', 'Revoke')}
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/if}

	<!-- Confirm modals -->
	<Modal bind:open={disableOpen} title={t('settings.atproto.disable_confirm_title', 'Disable Bluesky identity?')} description={t('settings.atproto.disable_confirm_desc', 'Your AT Protocol records stop syncing and your repo stops serving. Your DID and handle stay reserved — you can re-enable at any time. App passwords and connected apps stop working while disabled.')} size="sm">
		{#snippet children(close)}
			<div class="flex items-center justify-end gap-2">
				<button type="button" disabled={busy} onclick={close} class="rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
					{t('settings.action.cancel', 'Cancel')}
				</button>
				<button type="button" disabled={busy} onclick={handleDisable} class="rounded-[var(--radius)] bg-[var(--color-danger,#dc2626)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50">
					{busy ? t('common.submitting', 'Submitting…') : t('settings.atproto.disable', 'Disable')}
				</button>
			</div>
		{/snippet}
	</Modal>

	<Modal bind:open={revokePwOpen} title={t('settings.atproto.revoke_password_title', 'Revoke app password?')} description={t('settings.atproto.revoke_password_desc', 'Any client signed in with this password loses access immediately.')} size="sm">
		{#snippet children(close)}
			<div class="flex items-center justify-end gap-2">
				<button type="button" disabled={busy} onclick={close} class="rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
					{t('settings.action.cancel', 'Cancel')}
				</button>
				<button type="button" disabled={busy} onclick={handleRevokePassword} class="rounded-[var(--radius)] bg-[var(--color-danger,#dc2626)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50">
					{t('settings.atproto.revoke', 'Revoke')}
				</button>
			</div>
		{/snippet}
	</Modal>

	<Modal bind:open={revokeOAuthOpen} title={t('settings.atproto.revoke_session_title', 'Disconnect this app?')} description={t('settings.atproto.revoke_session_desc', 'Every OAuth token this client holds for your account is revoked — it must re-authorize to reconnect.')} size="sm">
		{#snippet children(close)}
			<div class="flex items-center justify-end gap-2">
				<button type="button" disabled={busy} onclick={close} class="rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
					{t('settings.action.cancel', 'Cancel')}
				</button>
				<button type="button" disabled={busy} onclick={handleRevokeOAuth} class="rounded-[var(--radius)] bg-[var(--color-danger,#dc2626)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50">
					{t('settings.atproto.revoke', 'Revoke')}
				</button>
			</div>
		{/snippet}
	</Modal>
</div>
