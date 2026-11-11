<script lang="ts" module>
	/**
	 * Idle cascade for dormant lazy cells. Dormant wrappers collapse to ~0
	 * height after hydration drops their SSR payload, so they all sit at the
	 * same scroll position — letting each IntersectionObserver mount directly
	 * would re-bundle every deferred block into one long task. Instead the
	 * queue mounts one cell per idle tick in document order; each mount grows
	 * the page so later cells land at their real position, and per-tick
	 * main-thread work stays below long-task thresholds. IO only reorders the
	 * queue (prioritize), never mounts.
	 */
	interface Pending {
		activate: () => void;
		active: () => boolean;
	}
	const pending: Pending[] = [];
	let scheduled = false;

	function schedule() {
		if (scheduled || pending.length === 0) return;
		scheduled = true;
		const run = () => {
			scheduled = false;
			while (pending.length) {
				const p = pending.shift()!;
				if (p.active()) continue; // already mounted
				p.activate();
				break;
			}
			schedule();
		};
		if (typeof requestIdleCallback === 'function') {
			requestIdleCallback(run, { timeout: 2000 });
		} else {
			setTimeout(run, 80);
		}
	}

	function prioritize(p: Pending) {
		const idx = pending.indexOf(p);
		if (idx > 0) {
			pending.splice(idx, 1);
			pending.unshift(p);
		}
		schedule();
	}
</script>

<script lang="ts">
	import { onMount } from 'svelte';
	import type { Snippet } from 'svelte';

	interface Props {
		children: Snippet;
		/** false → hydrate eagerly (identical SSR + client branch, no deferral). */
		lazy?: boolean;
		/** IntersectionObserver rootMargin — how far ahead to prioritize the mount. */
		rootMargin?: string;
		[attr: string]: unknown;
	}
	let { children, lazy = true, rootMargin = '800px 0px', ...rest }: Props = $props();

	let activated = $state(false);
	let host: HTMLDivElement;

	onMount(() => {
		if (!lazy) return;
		const item: Pending = {
			activate: () => (activated = true),
			active: () => activated
		};
		let io: IntersectionObserver | undefined;
		if (typeof IntersectionObserver === 'function') {
			io = new IntersectionObserver(
				(entries) => {
					if (entries.some((e) => e.isIntersecting)) {
						io?.disconnect();
						if (pending.indexOf(item) >= 0) {
							prioritize(item); // queued → bump to front, mount next idle tick
						} else {
							activated = true; // was display:none — mount immediately
						}
					}
				},
				{ rootMargin }
			);
			io.observe(host);
		}
		// display:none cells (responsive visibility) never intersect and would
		// stall the cascade — they stay IO-only until they gain a box.
		if (getComputedStyle(host).display !== 'none') {
			pending.push(item);
			schedule();
		}
		return () => {
			io?.disconnect();
			const idx = pending.indexOf(item);
			if (idx >= 0) pending.splice(idx, 1);
		};
	});
</script>

<div bind:this={host} {...rest}>
	{#if !lazy || activated || typeof window === 'undefined'}
		{@render children()}
	{/if}
</div>
