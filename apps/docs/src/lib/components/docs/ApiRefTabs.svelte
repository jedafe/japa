<script lang="ts">
	import { Tabs } from 'bits-ui';
	import { t } from '$lib/i18n';
	import CodeBlock from './CodeBlock.svelte';
	import MethodBadge from './MethodBadge.svelte';

	interface Props {
		method: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE';
		path: string;
		/** Request sample (cURL or JSON body). */
		requestSample?: string;
		requestLang?: string;
		/** Response sample (JSON). */
		responseSample?: string;
		responseLang?: string;
		/** End-to-end example. */
		exampleSample?: string;
		exampleLang?: string;
	}

	let {
		method,
		path,
		requestSample = '',
		requestLang = 'bash',
		responseSample = '',
		responseLang = 'json',
		exampleSample = '',
		exampleLang = 'bash'
	}: Props = $props();
</script>

<div class="api-ref">
	<div class="api-headline">
		<MethodBadge {method} />
		<code class="api-path">{path}</code>
	</div>

	<Tabs.Root value="request" class="tabs">
		<Tabs.List class="tab-list">
			<Tabs.Trigger value="request" class="tab-trigger">{t('api.request')}</Tabs.Trigger>
			<Tabs.Trigger value="response" class="tab-trigger">{t('api.response')}</Tabs.Trigger>
			<Tabs.Trigger value="example" class="tab-trigger">{t('api.example')}</Tabs.Trigger>
		</Tabs.List>

		<Tabs.Content value="request" class="tab-content">
			{#if requestSample}
				<CodeBlock code={requestSample} lang={requestLang} />
			{:else}
				<p class="tab-empty">—</p>
			{/if}
		</Tabs.Content>

		<Tabs.Content value="response" class="tab-content">
			{#if responseSample}
				<CodeBlock code={responseSample} lang={responseLang} />
			{:else}
				<p class="tab-empty">—</p>
			{/if}
		</Tabs.Content>

		<Tabs.Content value="example" class="tab-content">
			{#if exampleSample}
				<CodeBlock code={exampleSample} lang={exampleLang} />
			{:else}
				<p class="tab-empty">—</p>
			{/if}
		</Tabs.Content>
	</Tabs.Root>
</div>

<style>
	.api-ref {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.api-headline {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	.api-path {
		font-family: var(--font-mono);
		font-size: 0.875rem;
		color: var(--color-text);
		background: var(--color-surface-subtle);
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
	}
	:global(.tabs) { display: flex; flex-direction: column; gap: 0.5rem; }
	:global(.tab-list) {
		display: flex;
		gap: 0.25rem;
		border-bottom: 1px solid var(--color-border);
	}
	:global(.tab-trigger) {
		padding: 0.375rem 0.75rem;
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-muted);
		background: transparent;
		border: none;
		border-bottom: 2px solid transparent;
		cursor: pointer;
	}
	:global(.tab-trigger[data-state="active"]) {
		color: var(--color-primary);
		border-bottom-color: var(--color-primary);
	}
	:global(.tab-content) { padding-top: 0.25rem; }
	.tab-empty { color: var(--color-muted); font-size: 0.8125rem; padding: 0.5rem 0; }
</style>
