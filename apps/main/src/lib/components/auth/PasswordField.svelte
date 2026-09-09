<script lang="ts">
	// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
	// Password input with show/hide toggle and optional strength meter (signup).
	import { t } from '$lib/i18n/store.svelte';
	import { Eye, EyeOff } from 'lucide-svelte';
	import { passwordStrength } from '$lib/auth/validation';

	let {
		value = $bindable(),
		id,
		label,
		showStrength = false,
		error = '',
		autocomplete = 'current-password',
		placeholder = ''
	}: {
		value: string;
		id: string;
		label: string;
		showStrength?: boolean;
		error?: string;
		autocomplete?: HTMLInputElement['autocomplete'];
		placeholder?: string;
	} = $props();

	let visible = $state(false);

	const strength = $derived(showStrength ? passwordStrength(value) : 0);
	const strengthLabel = $derived(
		strength === 0
			? ''
			: strength === 1
				? 'auth.password.strength_weak'
				: strength === 2
					? 'auth.password.strength_fair'
					: strength === 3
						? 'auth.password.strength_good'
						: 'auth.password.strength_strong'
	);
	const strengthColor = $derived(
		strength <= 1 ? 'var(--color-muted)' : strength === 2 ? '#eab308' : 'var(--color-primary)'
	);
</script>

<div class="flex flex-col gap-1.5">
	<label for={id} class="text-sm font-medium text-[var(--color-text)]">{label}</label>
	<div class="relative">
		<input
			{id}
			type={visible ? 'text' : 'password'}
			bind:value
			{autocomplete}
			placeholder={placeholder || label}
			aria-label={label}
			aria-invalid={error ? 'true' : undefined}
			aria-describedby={error ? `${id}-error` : undefined}
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 pr-10 text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
		/>
		<button
			type="button"
			onclick={() => (visible = !visible)}
			class="absolute right-2.5 top-1/2 -translate-y-1/2 text-[var(--color-muted)] hover:text-[var(--color-text)]"
			aria-label={visible ? t('auth.fields.password_hide') : t('auth.fields.password_show')}
			tabindex="-1"
		>
			{#if visible}
				<EyeOff size={18} />
			{:else}
				<Eye size={18} />
			{/if}
		</button>
	</div>

	{#if showStrength && value}
		<div class="flex items-center gap-2">
			<div class="flex h-1.5 flex-1 gap-1">
				{#each Array(4) as _, i}
					<div
						class="flex-1 rounded-full transition-colors"
						style:background={i < strength ? strengthColor : 'var(--color-surface-subtle)'}
					></div>
				{/each}
			</div>
			<span class="text-xs text-[var(--color-muted)]">{t(strengthLabel)}</span>
		</div>
	{/if}

	{#if error}
		<p id="{id}-error" class="text-sm text-red-500">{t(error)}</p>
	{/if}
</div>
