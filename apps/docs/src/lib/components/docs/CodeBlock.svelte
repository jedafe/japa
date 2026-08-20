<script lang="ts">
	import { Copy, Check } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	interface Props {
		code: string;
		/** Language label, e.g. "bash", "json", "go". Empty hides the badge. */
		lang?: string;
	}

	let { code, lang = '' }: Props = $props();

	let copied = $state(false);
	let copyTimer: ReturnType<typeof setTimeout> | undefined = undefined;

	$effect(() => {
		return () => clearTimeout(copyTimer);
	});

	async function copy() {
		try {
			await navigator.clipboard.writeText(code);
			copied = true;
			copyTimer = setTimeout(() => (copied = false), 1500);
		} catch {
			// Clipboard may be unavailable (permissions); fail silently.
		}
	}
</script>

<div class="jc-code">
	<div class="jc-code-header">
		{#if lang}<span class="jc-code-lang">{lang}</span>{:else}<span></span>{/if}
		<button
			type="button"
			class="jc-code-copy"
			class:copied
			onclick={copy}
			aria-label={t('a11y.copy_code')}
		>
			{#if copied}
				<Check size={14} />
				<span>{t('a11y.code_copied')}</span>
			{:else}
				<Copy size={14} />
				<span>{t('a11y.copy_code')}</span>
			{/if}
		</button>
	</div>
	<pre><code>{code}</code></pre>
</div>
