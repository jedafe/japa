<script lang="ts">
	import { t } from '$lib/i18n';

	interface Props {
		title: string;
		onTitle: (v: string) => void;
	}

	let { title, onTitle }: Props = $props();
</script>

<div class="title-field">
	<input
		type="text"
		class="title-input"
		placeholder={t('create.placeholder.title')}
		value={title}
		oninput={(e) => onTitle(e.currentTarget.value)}
	/>
</div>

<style>
	.title-field {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.title-input {
		width: 100%;
		padding: 10px 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 1.125rem;
		font-weight: 600;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.title-input::placeholder {
		color: var(--color-muted);
		font-weight: 500;
	}
	.title-input:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
</style>
