<script lang="ts">
	interface Props {
		code: string;
		language?: string;
	}

	let { code, language = 'text' }: Props = $props();

	// Simple syntax highlighting via CSS classes
	// For production, integrate Prism.js or Shiki
	let highlightedCode = $derived(highlightSyntax(code, language));

	function highlightSyntax(code: string, lang: string): string {
		// Escape HTML
		let escaped = code
			.replace(/&/g, '&amp;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;');

		// Basic keyword highlighting for common languages
		if (lang === 'javascript' || lang === 'typescript' || lang === 'js' || lang === 'ts') {
			const keywords = ['const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while', 'class', 'import', 'export', 'from', 'async', 'await'];
			for (const kw of keywords) {
				escaped = escaped.replace(
					new RegExp(`\\b${kw}\\b`, 'g'),
					`<span class="keyword">${kw}</span>`
				);
			}
		}

		// String highlighting
		escaped = escaped.replace(/(["'])(?:(?=(\\?))\2.)*?\1/g, '<span class="string">$&</span>');

		// Comment highlighting
		escaped = escaped.replace(/\/\/.*$/gm, '<span class="comment">$&</span>');
		escaped = escaped.replace(/\/\*[\s\S]*?\*\//g, '<span class="comment">$&</span>');

		return escaped;
	}
</script>

<div class="code-block">
	{#if language && language !== 'text'}
		<div class="language-badge">{language}</div>
	{/if}

	<pre class="code-pre"><code>{@html highlightedCode}</code></pre>

	<button class="copy-btn" onclick={() => navigator.clipboard.writeText(code)}>
		Copy
	</button>
</div>

<style>
	.code-block {
		position: relative;
		margin: 1rem 0;
		border-radius: 0.5rem;
		overflow: hidden;
		background: var(--color-bg-code, #1e293b);
	}

	.language-badge {
		position: absolute;
		top: 0.5rem;
		right: 0.5rem;
		font-size: 0.75rem;
		padding: 0.125rem 0.5rem;
		background: var(--color-bg-muted, #334155);
		color: var(--color-text-muted, #94a3b8);
		border-radius: 0.25rem;
		text-transform: uppercase;
	}

	.code-pre {
		margin: 0;
		padding: 1rem;
		overflow-x: auto;
		font-family: 'Fira Code', 'Monaco', 'Consolas', monospace;
		font-size: 0.875rem;
		line-height: 1.6;
		color: var(--color-text-code, #e2e8f0);
	}

	.code-pre :global(.keyword) {
		color: #c084fc;
	}

	.code-pre :global(.string) {
		color: #86efac;
	}

	.code-pre :global(.comment) {
		color: #64748b;
		font-style: italic;
	}

	.copy-btn {
		position: absolute;
		bottom: 0.5rem;
		right: 0.5rem;
		padding: 0.25rem 0.75rem;
		font-size: 0.75rem;
		background: var(--color-bg-muted, #334155);
		color: var(--color-text-muted, #94a3b8);
		border: none;
		border-radius: 0.25rem;
		cursor: pointer;
		opacity: 0;
		transition: opacity 0.2s;
	}

	.code-block:hover .copy-btn {
		opacity: 1;
	}

	.copy-btn:hover {
		background: var(--color-bg-hover, #475569);
	}
</style>
