<script lang="ts">
	/**
	 * CodeMirror JSON editor wrapper for the API playground.
	 * Lazy-loads CodeMirror only in the browser (SSR-safe).
	 *
	 * Ref: §47 ROADMAP 04.06/04.07.
	 */
	import { onMount } from 'svelte';

	interface Props {
		value: string;
		placeholder?: string;
		/** Read-only mode (for response viewer). */
		readonly?: boolean;
		/** Min height in px. */
		minHeight?: number;
		/** Called on every change with the new document text. */
		onChange?: (value: string) => void;
	}

	let {
		value,
		placeholder = '',
		readonly = false,
		minHeight = 120,
		onChange
	}: Props = $props();

	let editorEl: HTMLDivElement | undefined = undefined;
	let view: import('@codemirror/view').EditorView | null = null;

	onMount(async () => {
		if (!editorEl) return;
		const { EditorView, keymap, lineNumbers } = await import('@codemirror/view');
		const { EditorState } = await import('@codemirror/state');
		const { defaultKeymap, history, historyKeymap } = await import('@codemirror/commands');
		const { json, jsonParseLinter } = await import('@codemirror/lang-json');
		const { linter } = await import('@codemirror/lint');

		const state = EditorState.create({
			doc: value,
			extensions: [
				lineNumbers(),
				history(),
				keymap.of([...defaultKeymap, ...historyKeymap]),
				json(),
				linter(jsonParseLinter()),
				EditorView.editable.of(!readonly),
				EditorView.theme({
					'&': {
						fontSize: '0.8125rem',
						background: 'var(--code-bg)',
						color: 'var(--code-text)',
						borderRadius: 'var(--radius)'
					},
					'.cm-gutters': {
						background: 'var(--code-bg)',
						border: 'none',
						color: 'var(--color-muted)'
					},
					'&.cm-focused': {
						outline: '1px solid var(--color-primary)'
					}
				}),
				EditorView.updateListener.of((update) => {
					if (update.docChanged && onChange) {
						onChange(update.state.doc.toString());
					}
				})
			]
		});

		view = new EditorView({ state, parent: editorEl });
	});

	// Sync external `value` prop changes back into the editor.
	$effect(() => {
		const next = value;
		if (!view || view.state.doc.toString() === next) return;
		view.dispatch({
			changes: { from: 0, to: view.state.doc.length, insert: next },
			scrollIntoView: false
		});
	});

	// Clean up on unmount
	$effect(() => {
		return () => {
			if (view) {
				view.destroy();
				view = null;
			}
		};
	});
</script>

<div
	class="cm-wrapper"
	style="min-height: {minHeight}px"
	bind:this={editorEl}
	data-placeholder={placeholder}
></div>

<style>
	.cm-wrapper {
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		overflow: hidden;
	}
	.cm-wrapper :global(.cm-editor) {
		min-height: inherit;
	}
	.cm-wrapper :global(.cm-scroller) {
		font-family: var(--font-mono);
	}
</style>
