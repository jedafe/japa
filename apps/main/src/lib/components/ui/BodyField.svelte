<script lang="ts">
	interface Props {
		id: string;
		label: string;
		body: string;
		onBody: (v: string) => void;
		placeholder?: string;
		rows?: number;
		disabled?: boolean;
		required?: boolean;
		maxlength?: number;
	}

	let {
		id,
		label,
		body,
		onBody,
		placeholder,
		rows = 8,
		disabled = false,
		required,
		maxlength
	}: Props = $props();

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
</script>

<div class="flex flex-col gap-1.5">
	<label for={id} class="text-sm font-medium text-[var(--color-text)]">
		{label}
		{#if required}<span class="text-red-500" aria-hidden="true">*</span>{/if}
	</label>
	<textarea
		{id}
		{rows}
		value={body}
		oninput={(e) => onBody(e.currentTarget.value)}
		{placeholder}
		class={inputClass}
		{disabled}
		{maxlength}
	></textarea>
</div>
