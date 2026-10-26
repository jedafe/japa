<script lang="ts">
	/**
	 * Lazy wrapper around CommentEditor (Section 49).
	 *
	 * CommentEditor pulls in the TipTap/ProseMirror stack (~1.9MB of chunks)
	 * which would otherwise sit in the critical path of every read-only
	 * content page — blog posts, stories, comparisons, forum threads —
	 * including for anonymous readers who can't comment at all. This stub
	 * renders a lightweight look-alike and dynamically imports the real
	 * editor on first interaction (hover/focus/click), or immediately when
	 * `eager`/`initialBody` indicates the user explicitly asked for it
	 * (reply/edit affordances).
	 */
	import { onMount, type Component } from 'svelte';
	import { Bold, Code, ImagePlus, Italic, Link as LinkIcon, Loader2, Strikethrough } from 'lucide-svelte';
	import { encodeTiptapDoc, type TipTapDoc } from '$lib/tiptap/serialize';

	interface Props {
		/** Placeholder text for the editor. */
		placeholder?: string;
		/** Initial content as base64 TipTap JSON (for edit mode). */
		initialBody?: string | null;
		/** Called whenever the editor content changes (with base64 TipTap JSON). */
		oninput?: (body: string) => void;
		/** CSS class for the editor container. */
		class?: string;
		/** Load the real editor on mount instead of waiting for interaction. */
		eager?: boolean;
	}

	let { placeholder = '', initialBody = null, oninput, class: cls = '', eager = false }: Props = $props();

	let EditorComponent = $state<Component<Props> | null>(null);
	let loading = $state(false);
	let stubText = $state('');
	let wrap = $state<HTMLDivElement | null>(null);
	let wantsFocus = $state(false);

	function load() {
		if (EditorComponent || loading) return;
		loading = true;
		import('./CommentEditor.svelte')
			.then((m) => {
				EditorComponent = m.default as Component<Props>;
			})
			.catch(() => {
				loading = false;
			});
	}

	// The user was mid-interaction when the real editor landed — move the
	// caret into it so they can keep typing without a second click. Only
	// for real interactions (click/focus/eager): a hover prefetch must not
	// steal focus.
	$effect(() => {
		if (!EditorComponent || !wrap || !wantsFocus) return;
		requestAnimationFrame(() => {
			wrap?.querySelector<HTMLElement>('.ProseMirror')?.focus();
		});
	});

	onMount(() => {
		if (eager || initialBody) {
			wantsFocus = true;
			load();
		}
	});

	// Carry over anything typed into the stub before the real editor lands,
	// wrapped in the base64 TipTap doc shape the editor and API expect.
	const stubDoc = $derived(
		stubText.trim()
			? encodeTiptapDoc({
					type: 'doc',
					content: [{ type: 'paragraph', content: [{ type: 'text', text: stubText }] }]
				} satisfies TipTapDoc)
			: null
	);

	// Keep the parent's draft state in sync while the stub is up so a fast
	// typist can hit Post before the heavy chunk arrives.
	$effect(() => {
		if (!EditorComponent && stubDoc && oninput) oninput(stubDoc);
	});

	const btnBase = 'grid place-items-center rounded p-1 text-[var(--color-muted)]';
	const toolbarIcons = [Bold, Italic, Strikethrough, Code, LinkIcon, ImagePlus];
</script>

{#if EditorComponent}
	<div bind:this={wrap} class="contents">
		<EditorComponent {placeholder} initialBody={initialBody ?? stubDoc} {oninput} class={cls} />
	</div>
{:else}
	<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] {cls}">
		<!-- Inert toolbar mirroring CommentEditor's geometry so the swap doesn't shift layout. -->
		<div class="flex items-center gap-0.5 border-b border-[var(--color-border)] px-1.5 py-1" aria-hidden="true">
			{#each toolbarIcons as Icon (Icon)}
				<button type="button" tabindex="-1" onclick={() => { wantsFocus = true; load(); }} class={btnBase}>
					<Icon size={14} />
				</button>
			{/each}
		</div>
		<div class="relative">
			<textarea
				bind:value={stubText}
				{placeholder}
				rows="2"
				class="block min-h-[2.5rem] w-full resize-none bg-transparent px-3 py-2 text-sm text-[var(--color-text)] outline-none placeholder:text-[var(--color-muted)]"
				onpointerenter={load}
				onpointerdown={() => { wantsFocus = true; load(); }}
				onfocus={() => { wantsFocus = true; load(); }}
			></textarea>
			{#if loading}
				<Loader2 size={14} class="absolute right-2 top-2 animate-spin text-[var(--color-muted)]" />
			{/if}
		</div>
	</div>
{/if}
