<script lang="ts">
	// Ref: japaconnect-engineering/sections/120-mcp-agent-distribution.md
	// (Login with JapaConnect — v1.4): OAuth sign-in is the primary connect
	// path; pasting an extension key stays as the manual fallback.
	import { siteName, DEFAULT_SITE_NAME } from '../../lib/site';
	import { getInstallId, type OAuthUser } from '../../lib/storage';
	import { siteUrl } from '../../lib/links';
	import { LoginCancelledError } from '../../lib/oauth';

	let brand = $state(DEFAULT_SITE_NAME);
	siteName().then((n) => (brand = n));
	let {
		connected,
		user,
		onConnected,
		onLogin,
		onDisconnected,
		onClose,
	}: {
		connected: boolean;
		user: OAuthUser | null;
		onConnected: (key: string) => Promise<void>;
		onLogin: () => Promise<void>;
		onDisconnected: () => Promise<void>;
		onClose: () => void;
	} = $props();

	let keyInput = $state('');
	let saving = $state(false);
	let signingIn = $state(false);
	let loginError = $state<string | null>(null);
	let installId = $state('');
	let copied = $state(false);

	// jc_live_ for production, jc_test_ for dev/staging keys.
	const validKey = $derived(
		keyInput.trim().startsWith('jc_live_') || keyInput.trim().startsWith('jc_test_'),
	);

	$effect(() => {
		getInstallId().then((id) => (installId = id));
	});

	async function save() {
		if (!validKey || saving) return;
		saving = true;
		try {
			await onConnected(keyInput.trim());
		} finally {
			saving = false;
		}
	}

	async function signIn() {
		if (signingIn) return;
		signingIn = true;
		loginError = null;
		try {
			await onLogin();
		} catch (e) {
			if (!(e instanceof LoginCancelledError)) {
				loginError = e instanceof Error ? e.message : 'Sign-in failed';
			}
		} finally {
			signingIn = false;
		}
	}

	async function copyInstallId() {
		try {
			await navigator.clipboard.writeText(installId);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {
			// clipboard API unavailable — the ID is visible for manual copy
		}
	}
</script>

<div class="settings" role="dialog" aria-label="Extension settings">
	<div class="settings-head">
		<h2>Connect</h2>
		<button class="ghost" onclick={onClose} aria-label="Close settings">✕</button>
	</div>

	{#if connected}
		{#if user}
			<p>
				This browser is signed in to {brand} as
				<strong>{user.name ?? user.preferred_username ?? user.email}</strong>{#if user.preferred_username}
					&nbsp;(<a href={siteUrl('/u/' + user.preferred_username)}>@{user.preferred_username}</a>){/if}.
			</p>
		{:else}
			<p>This browser is connected to your {brand} account.</p>
		{/if}
		<button class="danger" onclick={onDisconnected}>Disconnect this browser</button>
		<p class="hint">
			Manage or revoke connected apps under
			<a href={siteUrl('/settings')}>Settings → Connected apps</a> on the web — each browser's
			session shows as a <code>{brand} Browser Extension (…)</code> grant; pasted keys appear
			under API keys named <code>extension:*</code>.
		</p>
	{:else}
		<button class="primary" onclick={signIn} disabled={signingIn}>
			{signingIn ? 'Waiting for sign-in…' : `Sign in with ${brand}`}
		</button>
		{#if loginError}
			<p class="error" role="alert">{loginError}</p>
		{/if}
		<p class="hint">You'll be asked to authorize this browser on {siteUrl('')}.</p>

		<details class="manual-key">
			<summary>Use an extension key instead</summary>
			<ol class="steps">
				<li>
					Open <a href={siteUrl('/settings/api-keys')}>Settings → API keys</a> on japa.to and
					click <strong>Connect browser extension</strong>.
				</li>
				<li>Copy the key shown once — it is scoped read-only for this extension.</li>
				<li>Paste it below.</li>
			</ol>
			<label class="field">
				<span>Extension key</span>
				<input
					type="password"
					bind:value={keyInput}
					placeholder="jc_live_…"
					autocomplete="off"
					spellcheck="false"
				/>
				<span class="hint">The read-only key shown once in Settings → API keys.</span>
			</label>
			<button class="primary" onclick={save} disabled={saving || !validKey}>
				{saving ? 'Connecting…' : 'Connect'}
			</button>
			<details class="install-id">
				<summary>Advanced: install ID</summary>
				<p class="hint">
					Paste <code>{installId}</code> into the optional Install ID field on the mint form to
					name this browser's token recognizably.
					<button class="ghost" onclick={copyInstallId}>{copied ? 'Copied' : 'Copy'}</button>
				</p>
			</details>
		</details>
	{/if}
</div>
