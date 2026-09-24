<script lang="ts">
	import { getInstallId } from '../../lib/storage';
	import { siteUrl } from '../../lib/links';

	let {
		apiKey,
		onConnected,
		onDisconnected,
		onClose,
	}: {
		apiKey: string | null;
		onConnected: (key: string) => Promise<void>;
		onDisconnected: () => Promise<void>;
		onClose: () => void;
	} = $props();

	let keyInput = $state('');
	let saving = $state(false);
	let installId = $state('');
	let copied = $state(false);

	$effect(() => {
		getInstallId().then((id) => (installId = id));
	});

	async function save() {
		const key = keyInput.trim();
		if (!key.startsWith('jc_live_') || saving) return;
		saving = true;
		try {
			await onConnected(key);
		} finally {
			saving = false;
		}
	}

	async function copyInstallId() {
		await navigator.clipboard.writeText(installId);
		copied = true;
		setTimeout(() => (copied = false), 1500);
	}
</script>

<div class="settings" role="dialog" aria-label="Extension settings">
	<div class="settings-head">
		<h2>Connect</h2>
		<button class="ghost" onclick={onClose} aria-label="Close settings">✕</button>
	</div>

	{#if apiKey}
		<p>This browser is connected to your JapaConnect account.</p>
		<button class="danger" onclick={onDisconnected}>Disconnect this browser</button>
		<p class="hint">
			To revoke the key entirely, use your API keys page on the web — extension keys are named
			<code>extension:*</code>.
		</p>
	{:else}
		<ol class="steps">
			<li>
				Open <a href={siteUrl('/settings/developer')}>Settings → API keys</a> on japa.to and
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
		</label>
		<button class="primary" onclick={save} disabled={saving || !keyInput.trim().startsWith('jc_live_')}>
			{saving ? 'Connecting…' : 'Connect'}
		</button>
		<details class="install-id">
			<summary>Advanced: install ID</summary>
			<p class="hint">
				If the site asks for this browser's install ID when minting the key, it is
				<code>{installId}</code>.
				<button class="ghost" onclick={copyInstallId}>{copied ? 'Copied' : 'Copy'}</button>
			</p>
		</details>
	{/if}
</div>
