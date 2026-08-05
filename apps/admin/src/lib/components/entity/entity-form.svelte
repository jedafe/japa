<script lang="ts">
	import { clsx, type ClassValue } from 'clsx';
	import type { FieldDef } from './index';

	let {
		fields,
		value = $bindable({}),
		errors = {},
		disabled = false,
		onsubmit,
		class: className
	}: {
		fields: FieldDef[];
		value?: Record<string, any>;
		errors?: Record<string, string>;
		disabled?: boolean;
		onsubmit?: () => void;
		class?: ClassValue;
	} = $props();

	// Input types that map directly to a native <input type="..."> element.
	const inputTypes = new Set([
		'text', 'email', 'password', 'url', 'date', 'datetime-local', 'number', 'color'
	]);

	function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		onsubmit?.();
	}
</script>

<form class={clsx('flex flex-col gap-4', className)} onsubmit={handleSubmit}>
	{#each fields as field (field.key)}
		<div class="flex flex-col gap-1">
			<label for={field.key} class="text-sm font-medium" style="color: var(--color-foreground)">
				{field.label}{#if field.required}<span style="color: var(--color-danger)">*</span>{/if}
			</label>

			{#if field.type === 'textarea'}
				<textarea
					id={field.key}
					aria-label={field.label}
					placeholder={field.placeholder}
					disabled={disabled}
					bind:value={value[field.key]}
					class="rounded-lg border p-2 text-sm focus:outline-none focus:ring-2 disabled:cursor-not-allowed disabled:opacity-50"
					style="border-color: var(--color-border-input); background-color: var(--color-surface); color: var(--color-foreground)"
				></textarea>
			{:else if field.type === 'select'}
				<select
					id={field.key}
					aria-label={field.label}
					disabled={disabled}
					bind:value={value[field.key]}
					class="rounded-lg border p-2 text-sm focus:outline-none focus:ring-2 disabled:cursor-not-allowed disabled:opacity-50"
					style="border-color: var(--color-border-input); background-color: var(--color-surface); color: var(--color-foreground)"
				>
					{#each field.options ?? [] as option (option.value)}
						<option value={option.value}>{option.label}</option>
					{/each}
				</select>
			{:else if field.type === 'checkbox'}
				<input
					id={field.key}
					type="checkbox"
					aria-label={field.label}
					disabled={disabled}
					bind:checked={value[field.key]}
					class="h-4 w-4 rounded disabled:cursor-not-allowed disabled:opacity-50"
					style="accent-color: var(--color-primary)"
				/>
			{:else if field.type === 'color'}
				<input
					id={field.key}
					type="color"
					aria-label={field.label}
					disabled={disabled}
					bind:value={value[field.key]}
					class="h-10 w-full cursor-pointer rounded-lg border p-1 focus:outline-none focus:ring-2 disabled:cursor-not-allowed disabled:opacity-50"
					style="border-color: var(--color-border-input); background-color: var(--color-surface); color: var(--color-foreground)"
				/>
			{:else if inputTypes.has(field.type)}
				<input
					id={field.key}
					type={field.type}
					aria-label={field.label}
					placeholder={field.placeholder}
					disabled={disabled}
					min={field.min}
					max={field.max}
					step={field.step}
					bind:value={value[field.key]}
					class="rounded-lg border p-2 text-sm focus:outline-none focus:ring-2 disabled:cursor-not-allowed disabled:opacity-50"
					style="border-color: var(--color-border-input); background-color: var(--color-surface); color: var(--color-foreground)"
				/>
			{/if}

			{#if field.help}
				<span class="text-xs" style="color: var(--color-muted)">{field.help}</span>
			{/if}

			{#if errors[field.key]}
				<span class="text-xs" style="color: var(--color-danger)">{errors[field.key]}</span>
			{/if}
		</div>
	{/each}
</form>
