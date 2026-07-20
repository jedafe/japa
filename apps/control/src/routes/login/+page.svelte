<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, setToken } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	let email = $state('');
	let password = $state('');
	let loading = $state(false);
	let error = $state('');

	async function handleLogin() {
		if (!email || !password) return;
		if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
			error = 'Please enter a valid email address.';
			return;
		}
		loading = true;
		error = '';

		try {
			const result = await api<{ access_token: string }>('/api/v1/auth/login', {
				method: 'POST',
				body: { email, password }
			});

			if (result.ok) {
				if (result.data?.access_token) {
					const token = result.data.access_token;
					// SECURITY HARDENING (H4):
					// The auth backend returns only a raw JWT (`access_token`) and does
					// NOT set an httpOnly session cookie on login. Because `apiFetch`
					// already sends `credentials: 'include'`, a backend-issued httpOnly
					// session cookie would be forwarded automatically and we could drop
					// client-side JWT persistence entirely. Until that backend change
					// lands, the JWT MUST be persisted so the SPA can re-establish the
					// session on reload (see +layout.svelte, which reads it back).
					//
					// RESIDUAL RISK: a JWT in localStorage is reachable by any
					// XSS-injected script and can be exfiltrated. This is the minimal
					// safe change available without a backend session cookie — do not
					// remove the persistence below or the auth flow breaks.
					localStorage.setItem('jc_token', token);
					setToken(token);
					goto('/');
				} else {
					error = 'Invalid credentials.';
				}
			} else {
				error = result.error || 'Invalid credentials.';
			}
		} catch {
			error = 'Could not reach authentication service.';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>{t('login.title', 'Sign In')} — JapaConnect Control</title>
</svelte:head>

<div class="px-4">
	<div class="border-border-card bg-background-alt w-full max-w-sm rounded-lg border p-6">
		<h1 class="text-foreground mb-1 text-center text-xl font-bold tracking-tight">{t('control.brand', 'Control Panel')}</h1>
		<p class="text-muted-foreground mb-6 text-center text-sm">{t('login.title', 'Sign in to your account')}</p>

		{#if error}
			<div class="border-destructive bg-destructive/10 text-destructive mb-4 rounded-md border px-3 py-2 text-sm">
				{error}
			</div>
		{/if}

		<form onsubmit={(e) => { e.preventDefault(); handleLogin(); }} class="space-y-4">
			<div>
				<label for="email" class="text-foreground mb-1 block text-sm font-medium">{t('login.email', 'Email')}</label>
				<input
					id="email"
					type="email"
					required
					class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 w-full rounded-md border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					placeholder="admin@example.com"
					bind:value={email}
				/>
			</div>

			<div>
				<label for="password" class="text-foreground mb-1 block text-sm font-medium">{t('login.password', 'Password')}</label>
				<input
					id="password"
					type="password"
					required
					class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 w-full rounded-md border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					placeholder="••••••••"
					bind:value={password}
				/>
			</div>

			<button
				type="submit"
				class="bg-foreground text-background w-full rounded-md px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
				disabled={loading || !email || !password}
			>
				{loading ? t('login.signing_in', 'Signing in...') : t('login.submit', 'Sign In')}
			</button>
		</form>
	</div>
</div>
