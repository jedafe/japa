<script lang="ts">
	import { t } from '$lib/i18n';
	import AiBodyButton from './AiBodyButton.svelte';
	import AiDraftPanel from './AiDraftPanel.svelte';
	import { canUseAI } from './canUseAI';
	import type { FieldSchema } from './types';
	import type { SessionUser } from '$lib/types';

	interface Props {
		id: string;
		label: string;
		contentType: string;
		title: string;
		body: string;
		onBody: (v: string) => void;
		onTitle?: (v: string) => void;
		placeholder?: string;
		rows?: number;
		context?: Record<string, unknown>;
		fieldSchema?: FieldSchema[];
		disabled?: boolean;
		user?: SessionUser | null;
		features?: Record<string, boolean>;
		required?: boolean;
		maxlength?: number;
	}

	let {
		id,
		label,
		contentType,
		title,
		body,
		onBody,
		onTitle,
		placeholder,
		rows = 8,
		context,
		fieldSchema,
		disabled = false,
		user,
		features = {},
		required,
		maxlength
	}: Props = $props();

	let open = $state(false);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
</script>

<div class="flex flex-col gap-1.5">
	<div class="flex items-center justify-between">
		<label for={id} class="text-sm font-medium text-[var(--color-text)]">
			{label}
			{#if required}<span class="text-red-500" aria-hidden="true">*</span>{/if}
		</label>
		{#if canUseAI(user, features)}
			<AiBodyButton onclick={() => (open = true)} {disabled} />
		{/if}
	</div>
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

<AiDraftPanel
	{open}
	{contentType}
	{title}
	{body}
	{context}
	{fieldSchema}
	onApply={(payload) => {
		// Assist-tab payloads may carry metaTitle/metaDescription/tags instead
		// of a body — AiBodyField only owns the body/title fields, so those
		// keys are simply ignored here (CreateShell handles them).
		if (payload.body !== undefined) onBody(payload.body);
		if (payload.title && onTitle) onTitle(payload.title);
	}}
	onClose={() => (open = false)}
/>
