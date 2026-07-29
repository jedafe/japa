<script lang="ts">
	// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
	// Bits UI Dialog hosting LoginForm | SignupForm via an internal tab.
	// Triggered from the guest homepage header and any "Sign in to continue"
	// affordance in the shell. On success, invalidates all data and closes.
	import { Dialog } from 'bits-ui';
	import { untrack } from 'svelte';
	import { X } from 'lucide-svelte';
	import { t } from '$lib/i18n/store.svelte';
	import { invalidateAll } from '$app/navigation';
	import { goto } from '$app/navigation';
	import LoginForm from './LoginForm.svelte';
	import SignupForm from './SignupForm.svelte';

	let {
		open = $bindable(false),
		initialMode = 'login',
		redirect = '/'
	}: {
		open?: boolean;
		initialMode?: 'login' | 'signup';
		redirect?: string;
	} = $props();

	let mode = $state<'login' | 'signup'>(untrack(() => initialMode));

	async function onSuccess(target: string) {
		open = false;
		await invalidateAll();
		if (target && target !== '/') {
			await goto(target);
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay
			class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out"
		/>
		<Dialog.Content
			class="fixed left-1/2 top-1/2 z-50 w-full max-w-[460px] -translate-x-1/2 -translate-y-1/2 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-bg)] p-6 shadow-xl sm:p-8"
		>
			<Dialog.Close
				class="absolute right-4 top-4 text-[var(--color-muted)] hover:text-[var(--color-text)]"
			>
				<X size={20} />
			</Dialog.Close>

			<div class="mb-4 flex gap-1 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-1">
				<button
					type="button"
					onclick={() => (mode = 'login')}
					class="flex-1 rounded-[6px] px-3 py-1.5 text-sm font-medium transition"
					class:bg-[var(--color-surface)]={mode === 'login'}
					class:text-[var(--color-text)]={mode === 'login'}
					class:text-[var(--color-muted)]={mode !== 'login'}
				>
					{t('auth.login.submit')}
				</button>
				<button
					type="button"
					onclick={() => (mode = 'signup')}
					class="flex-1 rounded-[6px] px-3 py-1.5 text-sm font-medium transition"
					class:bg-[var(--color-surface)]={mode === 'signup'}
					class:text-[var(--color-text)]={mode === 'signup'}
					class:text-[var(--color-muted)]={mode !== 'signup'}
				>
					{t('auth.signup.submit')}
				</button>
			</div>

			{#if mode === 'login'}
				<LoginForm redirectTarget={redirect} {onSuccess} showSwitchLink={false} />
			{:else}
				<SignupForm redirectTarget="/onboarding" {onSuccess} showSwitchLink={false} />
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
