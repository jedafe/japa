<script lang="ts">
	// MediaLightbox — fullscreen media viewer ported from the main app (§23).
	// Pointer-event driven: pinch-zoom, pan-when-zoomed, swipe-to-navigate,
	// wheel zoom and double-tap/click zoom toggle on a single unified path.
	import { Dialog } from 'bits-ui';
	import { X, ChevronLeft, ChevronRight } from 'lucide-svelte';
	import type { EditorTranslator } from '../editor/types';

	export interface LightboxItem {
		id: string;
		url: string;
		alt_text?: string | null;
		caption?: string | null;
		filename?: string | null;
		mime_type?: string | null;
	}

	interface Props {
		open: boolean;
		items: LightboxItem[];
		index?: number;
		/** i18n translator injected by the host app. */
		t?: EditorTranslator;
	}

	const MIN_SCALE = 1;
	const MAX_SCALE = 5;
	const SWIPE_PX = 50;
	const DOUBLE_TAP_MS = 300;

	let { open = $bindable(), items, index = $bindable(0), t = (_k, fb) => fb ?? _k }: Props = $props();

	let scale = $state(1);
	let tx = $state(0);
	let ty = $state(0);
	let stageEl = $state<HTMLElement>();
	let imgEl = $state<HTMLImageElement>();
	let interacting = $state(false);

	const pointers = new Map<number, { x: number; y: number }>();
	let pinch: { dist: number; scale: number; tx: number; ty: number; midX: number; midY: number } | null = null;
	let pan: { x: number; y: number; tx: number; ty: number } | null = null;
	let swipe: { x: number; y: number } | null = null;
	let downPos: { x: number; y: number } | null = null;
	let lastTap = { t: 0, x: 0, y: 0 };

	const safeIndex = $derived(Math.max(0, Math.min(index, items.length - 1)));
	const current = $derived(items[safeIndex]);
	const isImage = $derived(!current?.mime_type || current.mime_type.startsWith('image/'));
	const caption = $derived(current?.caption || current?.alt_text || current?.filename || '');

	function clampScale(s: number): number {
		return Math.min(MAX_SCALE, Math.max(MIN_SCALE, s));
	}

	// Keep the image from being dragged out of view: allow panning only until
	// the scaled image edge reaches the stage edge on each axis.
	function clampPan() {
		if (!stageEl || !imgEl || scale <= 1) return;
		// offsetWidth/Height are layout metrics — unaffected by the transform.
		const uw = imgEl.offsetWidth;
		const uh = imgEl.offsetHeight;
		const maxX = Math.max(0, (uw * scale - stageEl.offsetWidth) / 2);
		const maxY = Math.max(0, (uh * scale - stageEl.offsetHeight) / 2);
		tx = Math.min(maxX, Math.max(-maxX, tx));
		ty = Math.min(maxY, Math.max(-maxY, ty));
	}

	function stagePoint(e: MouseEvent): { x: number; y: number } {
		const r = stageEl?.getBoundingClientRect();
		if (!r) return { x: 0, y: 0 };
		return { x: e.clientX - (r.left + r.width / 2), y: e.clientY - (r.top + r.height / 2) };
	}

	function zoomAt(px: number, py: number, nextScale: number) {
		const s = clampScale(nextScale);
		const k = s / scale;
		tx = px - (px - tx) * k;
		ty = py - (py - ty) * k;
		scale = s;
		if (scale <= 1) { tx = 0; ty = 0; }
		else clampPan();
	}

	function resetZoom() {
		scale = 1;
		tx = 0;
		ty = 0;
		pinch = null;
		pan = null;
		swipe = null;
		downPos = null;
	}

	function go(delta: number) {
		index = Math.min(items.length - 1, Math.max(0, index + delta));
		resetZoom();
	}

	function onPointerDown(e: PointerEvent) {
		if (!isImage) return;
		try {
			(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		} catch {
			// Synthetic or already-released pointers cannot be captured; safe to ignore.
		}
		pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
		interacting = true;
		if (pointers.size === 2) {
			const [a, b] = [...pointers.values()];
			pinch = {
				dist: Math.hypot(b.x - a.x, b.y - a.y),
				scale,
				tx,
				ty,
				midX: mid(a, b, 'x'),
				midY: mid(a, b, 'y')
			};
			swipe = null;
			pan = null;
			downPos = null;
		} else if (pointers.size === 1) {
			downPos = { x: e.clientX, y: e.clientY };
			if (scale > 1) pan = { x: e.clientX, y: e.clientY, tx, ty };
			else swipe = { x: e.clientX, y: e.clientY };
		}
	}

	function mid(a: { x: number; y: number }, b: { x: number; y: number }, axis: 'x' | 'y'): number {
		const r = stageEl?.getBoundingClientRect();
		const center = axis === 'x' ? (r ? r.left + r.width / 2 : 0) : (r ? r.top + r.height / 2 : 0);
		return (a[axis] + b[axis]) / 2 - center;
	}

	function onPointerMove(e: PointerEvent) {
		if (!pointers.has(e.pointerId)) return;
		pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
		if (pointers.size === 2 && pinch) {
			const [a, b] = [...pointers.values()];
			const dist = Math.hypot(b.x - a.x, b.y - a.y);
			const s = clampScale(pinch.scale * (dist / pinch.dist));
			// Keep the anchored image point under the moving pinch midpoint.
			const anchorX = (pinch.midX - pinch.tx) / pinch.scale;
			const anchorY = (pinch.midY - pinch.ty) / pinch.scale;
			tx = mid(a, b, 'x') - anchorX * s;
			ty = mid(a, b, 'y') - anchorY * s;
			scale = s;
			if (scale <= 1) { tx = 0; ty = 0; }
			else clampPan();
		} else if (pointers.size === 1 && pan) {
			tx = pan.tx + (e.clientX - pan.x);
			ty = pan.ty + (e.clientY - pan.y);
			clampPan();
		}
	}

	function onPointerEnd(e: PointerEvent) {
		const wasSwipe = swipe && pointers.size === 1 && scale <= 1;
		const endX = e.clientX;
		const endY = e.clientY;
		const moved = downPos ? Math.hypot(endX - downPos.x, endY - downPos.y) : Infinity;
		pointers.delete(e.pointerId);
		if (pointers.size < 2) pinch = null;
		if (pointers.size === 1) {
			// Finger lifted off a pinch — restart pan from the remaining finger.
			const [p] = pointers.values();
			pan = scale > 1 ? { x: p.x, y: p.y, tx, ty } : null;
		} else if (pointers.size === 0) {
			pan = null;
			interacting = false;
		}
		if (wasSwipe && swipe) {
			const dx = endX - swipe.x;
			const dy = endY - swipe.y;
			if (Math.abs(dx) > SWIPE_PX && Math.abs(dx) > Math.abs(dy) * 1.5) {
				go(dx < 0 ? 1 : -1);
				swipe = null;
				return;
			}
		}
		swipe = null;
		// Tap on empty stage area (not the image) closes the viewer.
		if (e.type === 'pointerup' && downPos && moved < 10 && scale <= 1 && e.target === stageEl) {
			open = false;
			downPos = null;
			return;
		}
		downPos = null;
		// Double-tap zoom toggle (touch) — requires a true tap on the image.
		if (
			e.pointerType !== 'mouse' &&
			e.type === 'pointerup' &&
			moved < 10 &&
			e.target !== stageEl
		) {
			const now = performance.now();
			if (now - lastTap.t < DOUBLE_TAP_MS && Math.hypot(endX - lastTap.x, endY - lastTap.y) < 30) {
				const p = stagePoint(e);
				zoomAt(p.x, p.y, scale > 1 ? 1 : 2.5);
				lastTap = { t: 0, x: 0, y: 0 };
			} else {
				lastTap = { t: now, x: endX, y: endY };
			}
		}
	}

	function onWheel(e: WheelEvent) {
		if (!isImage) return;
		e.preventDefault();
		const p = stagePoint(e);
		zoomAt(p.x, p.y, scale * Math.exp(-e.deltaY * 0.002));
	}

	function onDblClick(e: MouseEvent) {
		if (!isImage || e.target === stageEl) return;
		const p = stagePoint(e);
		zoomAt(p.x, p.y, scale > 1 ? 1 : 2.5);
	}

	function onKeydown(e: KeyboardEvent) {
		if (!open) return;
		if (e.key === 'ArrowRight') go(1);
		else if (e.key === 'ArrowLeft') go(-1);
		else if (e.key === '0') resetZoom();
	}

	$effect(() => {
		if (open) {
			resetZoom();
			if (items.length === 0) open = false;
		} else {
			// Close mid-gesture (e.g. Escape while dragging) — clear all tracking.
			pointers.clear();
			interacting = false;
		}
	});

	// Preload neighbors so prev/next doesn't wait on the network.
	$effect(() => {
		if (!open) return;
		for (const i of [safeIndex - 1, safeIndex + 1]) {
			const item = items[i];
			if (item?.mime_type?.startsWith('image/')) new Image().src = item.url;
		}
	});
</script>

<svelte:window onkeydown={onKeydown} />

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="lb-overlay" />
		<Dialog.Content class="lb-content">
			<Dialog.Title class="lb-sr-only">
				{caption || t('lightbox.title', 'Media viewer')}
			</Dialog.Title>
			<Dialog.Description class="lb-sr-only">
				{t('lightbox.hint', 'Use arrow keys to navigate, scroll or pinch to zoom')}
			</Dialog.Description>

			<!-- Top bar -->
			<div class="lb-topbar">
				<span class="lb-counter">
					{safeIndex + 1} / {items.length}
				</span>
				<Dialog.Close class="lb-close" aria-label={t('lightbox.close', 'Close')}>
					<X size={18} />
				</Dialog.Close>
			</div>

			<!-- Nav buttons -->
			{#if safeIndex > 0}
				<button
					type="button"
					class="lb-nav lb-nav-prev"
					onclick={() => go(-1)}
					aria-label={t('lightbox.prev', 'Previous')}
				>
					<ChevronLeft size={22} />
				</button>
			{/if}
			{#if safeIndex < items.length - 1}
				<button
					type="button"
					class="lb-nav lb-nav-next"
					onclick={() => go(1)}
					aria-label={t('lightbox.next', 'Next')}
				>
					<ChevronRight size={22} />
				</button>
			{/if}

			<!-- Stage -->
			{#if current}
				{#if isImage}
					<div
						bind:this={stageEl}
						class="lb-stage"
						onpointerdown={onPointerDown}
						onpointermove={onPointerMove}
						onpointerup={onPointerEnd}
						onpointercancel={onPointerEnd}
						onwheel={onWheel}
						ondblclick={onDblClick}
						role="presentation"
					>
						<img
							bind:this={imgEl}
							src={current.url}
							alt={caption}
							draggable="false"
							class="lb-img {interacting ? '' : 'lb-img-anim'}"
							style="transform: translate({tx}px, {ty}px) scale({scale});"
						/>
					</div>
				{:else if current.mime_type?.startsWith('video/')}
					<div class="lb-media-wrap">
						<!-- svelte-ignore a11y_media_has_caption -->
						<video src={current.url} controls playsinline class="lb-media"></video>
					</div>
				{:else if current.mime_type?.startsWith('audio/')}
					<div class="lb-media-wrap">
						<audio src={current.url} controls class="lb-audio"></audio>
					</div>
				{:else}
					<div class="lb-media-wrap">
						<a href={current.url} target="_blank" rel="noopener noreferrer" class="lb-file-link">
							{current.filename || t('lightbox.open_file', 'Open file')}
						</a>
					</div>
				{/if}
			{/if}

			<!-- Caption -->
			{#if caption}
				<div class="lb-caption-wrap">
					<span class="lb-caption">
						{caption}
					</span>
				</div>
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	:global(.lb-overlay) {
		position: fixed;
		inset: 0;
		z-index: 300;
		background: rgb(0 0 0 / 0.9);
	}
	:global(.lb-content) {
		position: fixed;
		inset: 0;
		z-index: 301;
		display: flex;
		flex-direction: column;
		outline: none;
	}
	:global(.lb-sr-only) {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}
	.lb-topbar {
		pointer-events: none;
		position: absolute;
		inset-inline: 0;
		top: 0;
		z-index: 10;
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 12px;
	}
	.lb-counter {
		border-radius: 999px;
		background: rgb(0 0 0 / 0.5);
		padding: 4px 10px;
		font-size: 0.75rem;
		color: rgb(255 255 255 / 0.8);
	}
	:global(.lb-close) {
		pointer-events: auto;
		margin-left: auto;
		display: grid;
		place-items: center;
		height: 36px;
		width: 36px;
		border: none;
		border-radius: 50%;
		background: rgb(0 0 0 / 0.5);
		color: rgb(255 255 255 / 0.8);
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}
	:global(.lb-close:hover) {
		background: rgb(0 0 0 / 0.7);
		color: #fff;
	}
	.lb-nav {
		position: absolute;
		top: 50%;
		z-index: 10;
		display: grid;
		place-items: center;
		height: 40px;
		width: 40px;
		transform: translateY(-50%);
		border: none;
		border-radius: 50%;
		background: rgb(0 0 0 / 0.5);
		color: rgb(255 255 255 / 0.8);
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}
	.lb-nav:hover {
		background: rgb(0 0 0 / 0.7);
		color: #fff;
	}
	.lb-nav-prev { left: 8px; }
	.lb-nav-next { right: 8px; }
	.lb-stage {
		display: flex;
		height: 100%;
		width: 100%;
		align-items: center;
		justify-content: center;
		overflow: hidden;
		touch-action: none;
		user-select: none;
	}
	.lb-img {
		max-height: 100%;
		max-width: 100%;
		object-fit: contain;
	}
	.lb-img-anim {
		transition: transform 0.15s;
	}
	.lb-media-wrap {
		display: flex;
		height: 100%;
		width: 100%;
		align-items: center;
		justify-content: center;
		padding: 16px;
	}
	.lb-media {
		max-height: 100%;
		max-width: 100%;
	}
	.lb-audio {
		width: 100%;
		max-width: 28rem;
	}
	.lb-file-link {
		border-radius: var(--radius, 6px);
		background: rgb(255 255 255 / 0.1);
		padding: 8px 16px;
		font-size: 0.875rem;
		color: #fff;
	}
	.lb-file-link:hover {
		background: rgb(255 255 255 / 0.2);
	}
	.lb-caption-wrap {
		pointer-events: none;
		position: absolute;
		inset-inline: 0;
		bottom: 0;
		display: flex;
		justify-content: center;
		padding: 16px;
	}
	.lb-caption {
		max-width: 36rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		border-radius: 999px;
		background: rgb(0 0 0 / 0.5);
		padding: 6px 16px;
		font-size: 0.875rem;
		color: rgb(255 255 255 / 0.9);
	}
</style>
