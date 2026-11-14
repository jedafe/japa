<script lang="ts">
	/**
	 * Floating TOC affordance — a circular button at the bottom-left of the
	 * viewport that appears once the reader scrolls past the inline TocBlock
	 * (IntersectionObserver on `watch`; scrollY fallback when there is no
	 * inline block). Click opens a responsive dialog: right-side slide-in on
	 * sm+ (desktop/pad), bottom sheet on mobile (<sm, matching the app's
	 * mobile chrome boundary).
	 *
	 * Position notes: `start-4` keeps it on the opposite corner from the
	 * Japa AI FAB in both LTR and RTL. The <sm bottom offset clears both the
	 * shell tab bar (~4.5rem) on shell pages and ReactionRail's fixed bar on
	 * content singles; above sm neither exists.
	 */
	import { Dialog } from 'bits-ui';
	import { List, X } from 'lucide-svelte';
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
	// the jump must wait for the close animation to release it — whichever
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

	function jump(_e: MouseEvent, id: string) {
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
</script>

{#if items.length > 0}
	<button
		type="button"
		onclick={() => (open = true)}
		aria-label={t('toc.open', 'Table of contents')}
		title={t('toc.on_this_page', 'On this page')}
		class="fixed start-4 z-40 grid h-12 w-12 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-lg transition-all duration-200 hover:bg-[var(--color-primary-hover)] bottom-[calc(4.5rem+env(safe-area-inset-bottom,0px))] sm:bottom-6 {visible
			? 'translate-y-0 opacity-100'
			: 'pointer-events-none translate-y-3 opacity-0'}"
	>
		<List size={20} />
	</button>

	<Dialog.Root bind:open {onOpenChangeComplete}>
		<Dialog.Portal>
			<Dialog.Overlay
				class="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out"
			/>
			<Dialog.Content
				class="fixed inset-x-0 bottom-0 z-50 flex max-h-[78dvh] flex-col rounded-t-[16px] border-t border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl transition-transform duration-300 ease-out data-[state=open]:translate-y-0 data-[state=closed]:translate-y-full sm:inset-y-0 sm:end-0 sm:start-auto sm:bottom-auto sm:h-full sm:max-h-none sm:w-full sm:max-w-md sm:rounded-none sm:border-s sm:border-t-0 sm:data-[state=open]:translate-x-0 sm:data-[state=closed]:translate-x-full sm:data-[state=closed]:translate-y-0 sm:rtl:data-[state=closed]:-translate-x-full"
			>
				<div class="mx-auto mt-2 h-1 w-10 shrink-0 rounded-full bg-[var(--color-border)] sm:hidden" aria-hidden="true"></div>
				<div class="flex items-center justify-between border-b border-[var(--color-border)] px-5 py-3.5">
					<Dialog.Title class="flex items-center gap-2 text-base font-semibold text-[var(--color-text)]">
						<List size={18} class="text-[var(--color-primary-text)]" aria-hidden="true" />
						{t('toc.on_this_page', 'On this page')}
					</Dialog.Title>
					<Dialog.Close
						class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						aria-label={t('common.close', 'Close')}
					>
						<X size={16} />
					</Dialog.Close>
				</div>

				<div class="min-h-0 flex-1 overflow-y-auto px-5 py-4">
					<TocItems {items} onjump={jump} />
				</div>
			</Dialog.Content>
		</Dialog.Portal>
	</Dialog.Root>
{/if}
