<script lang="ts">
	// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (OAuth social login)
	// Renders a button per provider the backend reports as configured via
	// GET /api/v1/auth/oauth/providers (env credentials or Control-managed
	// client entries). Falls back to the original Google/Apple/Facebook set
	// when the endpoint can't be reached so the buttons degrade gracefully.
	import { t } from '$lib/i18n/store.svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import { onMount } from 'svelte';

	let { redirect }: { redirect?: string } = $props();

	const KNOWN = ['google', 'apple', 'facebook', 'github'] as const;
	type Provider = (typeof KNOWN)[number];

	let providers = $state<Provider[]>([]);
	let loading = $state<string | null>(null);
	let error = $state('');

	onMount(async () => {
		const res = await api.get<string[]>('/auth/oauth/providers');
		const list = (res.data ?? []).filter((p): p is Provider =>
			(KNOWN as readonly string[]).includes(p)
		);
		// Endpoint unreachable or empty: keep the previous static set so the
		// feature doesn't silently vanish on a transient failure.
		providers = list.length > 0 ? list : ['google', 'apple', 'facebook'];
	});

	async function initiate(provider: string) {
		error = '';
		loading = provider;
		try {
			const target = redirect ?? page.url.pathname + page.url.search;
			const res = await fetch(`/api/auth/oauth?provider=${provider}&redirect=${encodeURIComponent(target)}`);
			const data = await res.json();
			if (!res.ok) {
				error = data?.error?.code ?? 'oauth_error';
				return;
			}
			window.location.assign(data.redirect_url);
		} catch {
			error = 'network_error';
		} finally {
			loading = null;
		}
	}
</script>

{#if error}
	<p class="text-sm text-red-500" role="alert">{t('auth.errors.oauth_' + error, t('auth.errors.generic'))}</p>
{/if}

{#if providers.length > 0}
<div class="grid grid-cols-3 gap-2.5">
	{#if providers.includes('google')}
	<button
		type="button"
		onclick={() => initiate('google')}
		disabled={loading !== null}
		class="flex items-center justify-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-2.5 text-xs font-medium text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)] disabled:opacity-50 sm:px-3 sm:text-sm"
		aria-label={t('auth.oauth.continue_with_google')}
	>
		<svg class="h-5 w-5 shrink-0" viewBox="0 0 24 24" aria-hidden="true">
			<path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
			<path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
			<path fill="#FBBC05" d="M5.84 14.09c-.22.66-.35 1.36-.35 2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
			<path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
		</svg>
		<span class="hidden truncate sm:inline">{loading === 'google' ? '...' : t('auth.oauth.google', 'Google')}</span>
	</button>
	{/if}

	{#if providers.includes('apple')}
	<button
		type="button"
		onclick={() => initiate('apple')}
		disabled={loading !== null}
		class="flex items-center justify-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-2.5 text-xs font-medium text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)] disabled:opacity-50 sm:px-3 sm:text-sm"
		aria-label={t('auth.oauth.continue_with_apple')}
	>
		<svg class="h-5 w-5 shrink-0" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
			<path d="M17.05 20.28c-.98.95-2.05.8-3.08.35-1.09-.46-2.09-.48-3.24 0-1.44.62-2.2.44-3.06-.35C2.79 15.25 3.51 7.59 9.05 7.31c1.35.07 2.29.74 3.08.8 1.18-.24 2.31-.93 3.57-.84 1.51.12 2.65.72 3.4 1.8-3.12 1.87-2.38 5.98.48 7.13-.57 1.5-1.31 2.99-2.54 4.09l.01-.01zM12.03 7.25c-.15-2.23-1.66-4.07-3.74-4.25.29 2.58-2.34 4.5-3.74 4.25z"/>
		</svg>
		<span class="hidden truncate sm:inline">{loading === 'apple' ? '...' : t('auth.oauth.apple', 'Apple')}</span>
	</button>
	{/if}

	{#if providers.includes('facebook')}
	<button
		type="button"
		onclick={() => initiate('facebook')}
		disabled={loading !== null}
		class="flex items-center justify-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-2.5 text-xs font-medium text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)] disabled:opacity-50 sm:px-3 sm:text-sm"
		aria-label={t('auth.oauth.continue_with_facebook')}
	>
		<svg class="h-5 w-5 shrink-0" viewBox="0 0 24 24" fill="#1877F2" aria-hidden="true">
			<path d="M24 12.07C24 5.41 18.63 0 12 0S0 5.41 0 12.07C0 18.1 4.39 23.1 10.13 24v-8.44H7.08v-3.49h3.05V9.41c0-3.02 1.79-4.69 4.53-4.69 1.31 0 2.68.24 2.68.24v2.97h-1.51c-1.49 0-1.96.93-1.96 1.89v2.25h3.33l-.53 3.49h-2.8V24C19.61 23.1 24 18.1 24 12.07z"/>
		</svg>
		<span class="hidden truncate sm:inline">{loading === 'facebook' ? '...' : t('auth.oauth.facebook', 'Facebook')}</span>
	</button>
	{/if}

	{#if providers.includes('github')}
	<button
		type="button"
		onclick={() => initiate('github')}
		disabled={loading !== null}
		class="flex items-center justify-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-2.5 text-xs font-medium text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)] disabled:opacity-50 sm:px-3 sm:text-sm"
		aria-label={t('auth.oauth.continue_with_github')}
	>
		<svg class="h-5 w-5 shrink-0" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
			<path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"/>
		</svg>
		<span class="hidden truncate sm:inline">{loading === 'github' ? '...' : t('auth.oauth.github', 'GitHub')}</span>
	</button>
	{/if}
</div>
{/if}
