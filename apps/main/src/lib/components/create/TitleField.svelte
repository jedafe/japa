<script lang="ts">
	import { t } from '$lib/i18n';
	import { LiteralSuggest } from '$lib/components/ui';

	interface Props {
		title: string;
		onTitle: (v: string) => void;
	}

	let { title, onTitle }: Props = $props();
</script>

<div class="title-field">
	<!-- {{ token picker — same literal autocomplete the body editor offers,
	     for plain-text fields. Inserts raw {{Token}} source; the server
	     resolves it at the response boundary (§14). -->
	<LiteralSuggest
		value={title}
		onvalue={onTitle}
		placeholder={t('create.placeholder.title')}
		class="title-input"
	/>
</div>

<style>
	.title-field {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	/* The input renders inside LiteralSuggest — :global reaches it. */
	.title-field :global(.title-input) {
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
	.title-field :global(.title-input)::placeholder {
		color: var(--color-muted);
		font-weight: 500;
	}
	.title-field :global(.title-input):focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
</style>
