<script lang="ts">
	import { setToken } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	let email = $state('');
	let password = $state('');
	let loading = $state(false);
	let error = $state('');

	async function handleLogin() {
		if (!email || !password) return;
		if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
			error = t('login.invalid_email', 'Please enter a valid email address.');
			return;
		}
		loading = true;
		error = '';

		try {
			// Call login endpoint directly with fetch to get the httpOnly
			// jc_refresh cookie set by the backend. The access token is
			// stored in-memory only (not localStorage) to prevent XSS
			// token theft. On reload, the layout calls /auth/refresh to
			// re-establish the session using the httpOnly cookie.
			// Ref: sections/02-authentication-sessions.md (Token model)
			const res = await fetch('/api/v1/auth/login', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				credentials: 'include',
				body: JSON.stringify({ email, password })
			});

			const text = await res.text();
			let body: { data?: { access_token?: string }; error?: { message?: string } } = {};
			if (text) {
				try {
					body = JSON.parse(text);
				} catch {
					body = {};
				}
			}

			if (res.ok && body.data?.access_token) {
				setToken(body.data.access_token);
				// Hard reload so the root layout re-mounts and onMount
				// re-runs with the fresh access token (fetches /me +
				// /me/permissions). Client-side goto() would NOT re-run
				// onMount since the layout is already mounted.
				window.location.href = '/';
			} else if (res.ok) {
				error = body.error?.message || t('login.invalid_credentials', 'Invalid credentials.');
			} else {
				const status = res.status ? ` (${res.status})` : '';
				error = body.error?.message || `Login failed${status}`;
			}
		} catch (e) {
			error = e instanceof Error
				? e.message
				: t('login.auth_service_unreachable', 'Could not reach authentication service.');
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>{t('control.login.title', 'Sign In')} — JapaConnect Control</title>
</svelte:head>

<div class="px-4">
	<div class="border-border-card bg-background-alt w-full max-w-sm rounded-lg border p-6">
		<h1 class="text-foreground mb-1 text-center text-xl font-bold tracking-tight">{t('control.brand', 'Control Panel')}</h1>
		<p class="text-muted-foreground mb-6 text-center text-sm">{t('control.login.title', 'Sign in to the control panel')}</p>

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
