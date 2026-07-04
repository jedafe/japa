<script lang="ts">
	import { Mail, Loader2, ArrowLeft } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	let email = $state('');
	let error = $state('');
	let loading = $state(false);
	let sent = $state(false);

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';
		loading = true;

		try {
			await api.forgotPassword(email);
			sent = true;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Network error. Please try again.';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Forgot Password | JapaConnect</title>
	<meta name="description" content="Reset your JapaConnect account password." />
</svelte:head>

<main class="mx-auto max-w-md px-6 py-16">
	<div class="rounded-xl border p-8" style="border-color: var(--color-border); background: var(--color-surface);">
		{#if sent}
			<div class="text-center">
				<div
					class="mx-auto flex h-12 w-12 items-center justify-center rounded-full"
					style="background-color: color-mix(in srgb, var(--color-success, var(--color-primary)) 15%, transparent)"
				>
					<Mail class="h-6 w-6" style="color: var(--color-primary)" />
				</div>
				<h1 class="mt-4 text-2xl font-bold" style="color: var(--color-foreground)">{t('auth.check_email')}</h1>
				<p class="mt-2 text-sm" style="color: var(--color-muted)">
					{t('auth.reset_link_sent')}
				</p>
				<a
					href="/login"
					class="mt-6 inline-flex items-center gap-2 text-sm font-medium hover:underline"
					style="color: var(--color-primary)"
				>
					<ArrowLeft class="h-4 w-4" />
					{t('auth.back_to_sign_in')}
				</a>
			</div>
		{:else}
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{t('auth.forgot_password')}</h1>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{t('auth.reset_instructions')}
			</p>

			{#if error}
				<div
					class="mt-4 rounded-lg border px-4 py-3 text-sm"
					style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
				>
					{error}
				</div>
			{/if}

			<form onsubmit={handleSubmit} class="mt-6 space-y-4">
				<div>
					<label for="email" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t('auth.email_label')}
					</label>
					<div class="relative">
						<Mail
							class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
							style="color: var(--color-muted)"
						/>
						<input
							id="email"
							type="email"
							required
							bind:value={email}
							placeholder="you@example.com"
							class="w-full rounded-lg border py-2.5 pl-10 pr-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
							style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						/>
					</div>
				</div>

				<button
					type="submit"
					disabled={loading}
					class="flex w-full items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background-color: var(--color-primary)"
				>
					{#if loading}
						<Loader2 class="h-4 w-4 animate-spin" />
					{:else}
						{t('auth.send_reset_link')}
					{/if}
				</button>
			</form>

			<div class="mt-6 flex items-center justify-between text-sm" style="color: var(--color-muted)">
				<a href="/login" class="inline-flex items-center gap-1 font-medium hover:underline" style="color: var(--color-primary)">
					<ArrowLeft class="h-3.5 w-3.5" />
					{t('auth.back_to_sign_in')}
				</a>
				<a href="/register" class="font-medium hover:underline" style="color: var(--color-primary)">
					{t('auth.create_account')}
				</a>
			</div>
		{/if}
	</div>
</main>
