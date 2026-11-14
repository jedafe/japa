<script lang="ts">
	/**
	 * Floating TOC affordance — a circular button at the bottom-left of the
	 * viewport that appears once the reader scrolls past the inline TocBlock
	 * (IntersectionObserver on `watch`; scrollY fallback when there is no
	 * inline block). Click opens the shared centered Modal with the rail-style
	 * TocItems list.
	 *
	 * Position notes: `toc-fab` + the app.css rules offset it clear of the
	 * shell sidenav (and the left ad rail) on ≥768px viewports; on mobile it
	 * stays at `start-4` — opposite corner from the Japa AI FAB in both LTR
	 * and RTL — raised above the shell tab bar / ReactionRail fixed bar.
	 */
	import { List } from 'lucide-svelte';
	import { Modal } from '#lib/components/ui/index.js';
	import { t } from '#lib/i18n/index.js';
	import TocItems from './TocItems.svelte';
	import { scrollToTocItem, type TocItem } from '#lib/utils/toc.js';

	let {
		items,
		watch = null
	}: {
		items: TocItem[];
		/** Inline TOC element — the FAB shows once it scrolls above the viewport. */
		watch?: HTMLElement | null;
	} = $props();

	let open = $state(false);
	let visible = $state(false);
	// Item click inside the dialog: the body is scroll-locked while open, so
	// the jump must wait for the close transition to release it — whichever
	// of onOpenChangeComplete / the safety timeout fires first wins.
	let pendingScrollId = $state('');
	let pendingTimer: ReturnType<typeof setTimeout> | undefined;

	function flushPendingScroll() {
		const id = pendingScrollId;
		if (!id) return;
		pendingScrollId = '';
		clearTimeout(pendingTimer);
		scrollToTocItem(id);
	}

	function jump(e: MouseEvent, id: string) {
		// The anchor's default nav must not run — it would push a history
		// entry and try to jump while the dialog still scroll-locks the
		// body. The deferred scrollToTocItem is the only navigation.
		e.preventDefault();
		pendingScrollId = id;
		open = false;
		pendingTimer = setTimeout(flushPendingScroll, 450);
	}

	function onOpenChangeComplete(isOpen: boolean) {
		if (!isOpen) flushPendingScroll();
	}

	$effect(() => {
		const el = watch;
		if (!el || typeof IntersectionObserver === 'undefined') {
			const onScroll = () => {
				visible = window.scrollY > 400;
			};
			onScroll();
			window.addEventListener('scroll', onScroll, { passive: true });
			return () => window.removeEventListener('scroll', onScroll);
		}
		const io = new IntersectionObserver(
			([entry]) => {
				visible = !entry.isIntersecting && entry.boundingClientRect.bottom < 0;
			},
			{ threshold: 0 }
		);
		io.observe(el);
		return () => io.disconnect();
	});

	// A pending jump whose component unmounts first — the timer handle is
	// released here; scrollToTocItem's null-target guard covers the rest.
	$effect(() => () => clearTimeout(pendingTimer));
</script>

{#if items.length > 0}
	<button
		type="button"
		onclick={() => (open = true)}
		aria-label={t('toc.open', 'Table of contents')}
		aria-hidden={!visible}
		tabindex={visible ? 0 : -1}
		inert={!visible}
		title={t('toc.on_this_page', 'On this page')}
		class="toc-fab fixed start-4 z-40 grid h-12 w-12 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-lg transition-all duration-200 hover:bg-[var(--color-primary-hover)] bottom-[calc(4.5rem+env(safe-area-inset-bottom,0px))] sm:bottom-6 {visible
			? 'translate-y-0 opacity-100'
			: 'pointer-events-none translate-y-3 opacity-0'}"
	>
		<List size={20} />
	</button>

	<Modal
		bind:open
		title={t('toc.on_this_page', 'On this page')}
		icon={List}
		size="max-w-md"
		{onOpenChangeComplete}
	>
		{#snippet children()}
			<TocItems {items} onjump={jump} />
		{/snippet}
	</Modal>
{/if}
