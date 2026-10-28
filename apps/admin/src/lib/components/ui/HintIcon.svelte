<script lang="ts">
	let { text, id }: { text: string; id?: string } = $props();
</script>

<span class="hint-icon" tabindex="0" role="button" aria-label="Field hint">
	<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
		<circle cx="12" cy="12" r="10" />
		<path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3" />
		<line x1="12" y1="17" x2="12.01" y2="17" />
	</svg>
	<span class="hint-tip" role="tooltip" {id}>{text}</span>
</span>

<style>
	.hint-icon {
		display: inline-flex;
		float: inline-end;
		margin-inline-start: auto;
		cursor: help;
		color: var(--color-muted, #6b7280);
		position: relative;
	}
	.hint-tip {
		position: absolute;
		z-index: 60;
		bottom: calc(100% + 8px);
		left: 50%;
		transform: translateX(-50%);
		width: max-content;
		max-width: 16rem;
		padding: 0.375rem 0.625rem;
		border-radius: 0.5rem;
		background: #111827;
		color: #f9fafb;
		font-size: 0.75rem;
		font-weight: 400;
		line-height: 1.35;
		text-align: start;
		white-space: normal;
		opacity: 0;
		visibility: hidden;
		transition: opacity 0.15s ease;
		pointer-events: none;
	}
	.hint-icon:hover .hint-tip,
	.hint-icon:focus-visible .hint-tip,
	.hint-icon:focus .hint-tip {
		opacity: 1;
		visibility: visible;
	}
</style>
