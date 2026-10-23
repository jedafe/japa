<script lang="ts">
	// Modal — shared wrapper around bits-ui Dialog for standard centered
	// modals. Use this instead of wiring Dialog.Root/Portal/Overlay/Content
	// by hand; bespoke surfaces (drawers, bottom sheets, fullscreen viewers)
	// may still use bits-ui directly.
	import { Dialog } from 'bits-ui';
	import { X } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import type { IconComponent } from './icon';

	interface Props {
		/** Bindable open state. */
		open?: boolean;
		/** Fires when bits-ui wants to change open (Esc, overlay click). */
		onOpenChange?: (open: boolean) => void;
		/** Visible dialog title — renders the header row with a close button.
		 * Pass a snippet for rich title content (e.g. title + count badge). */
		title?: string | Snippet;
		/** Icon shown left of the title. */
		icon?: IconComponent;
		/** Screen-reader-only title when no visible title is wanted. */
		ariaTitle?: string;
		/** Muted description rendered under the header. */
		description?: string | Snippet;
		/** Max-width preset for the panel, or a raw `max-w-*` class. */
		size?: 'sm' | 'md' | 'lg' | 'xl' | `max-w-${string}`;
		/** z-index applied to overlay + content (content paints after overlay). */
		zIndex?: number;
		/** Hide the default header entirely (title/close) — for custom headers. */
		hideHeader?: boolean;
		/** Apply the default panel padding (p-5). Set false for edge-to-edge
		 * layouts like list modals with a bordered header row. */
		padded?: boolean;
		/** Extra classes merged onto Dialog.Content. */
		class?: string;
		/** Extra classes merged onto Dialog.Overlay. */
		overlayClass?: string;
		/** Custom header replacing the default title+close row; receives `close()`. */
		header?: Snippet<[close: () => void]>;
		/** Trigger content — rendered inside Dialog.Trigger before the portal. */
		trigger?: Snippet;
		/** Classes applied to the trigger button. */
		triggerClass?: string;
		/** Accessible label for the close button. */
		closeLabel?: string;
		/** Body — receives `close()` so in-body buttons can dismiss the dialog
		 * without importing bits-ui (works for bound and unbound `open`). */
		children?: Snippet<[close: () => void]>;
	}

	let {
		open = $bindable(false),
		onOpenChange,
		title,
		icon: Icon,
		ariaTitle,
		description,
		size = 'md',
		zIndex = 50,
		hideHeader = false,
		padded = true,
		class: className = '',
		overlayClass = '',
		header,
		trigger,
		triggerClass = '',
		closeLabel = 'Close',
		children
	}: Props = $props();

	const sizes: Record<string, string> = {
		sm: 'max-w-sm',
		md: 'max-w-md',
		lg: 'max-w-lg',
		xl: 'max-w-xl'
	};

	// Handles both bound (`bind:open`) and unbound (`open` + `onOpenChange`)
	// usage — mirrors what bits-ui's own Dialog.Close does internally.
	function requestClose() {
		open = false;
		onOpenChange?.(false);
	}
</script>

<Dialog.Root bind:open {onOpenChange}>
	{#if trigger}
		<Dialog.Trigger type="button" class={triggerClass}>
			{@render trigger()}
		</Dialog.Trigger>
	{/if}
	<Dialog.Portal>
		<Dialog.Overlay
			class="fixed inset-0 bg-black/50 backdrop-blur-sm data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 {overlayClass}"
			style="z-index: {zIndex}"
		/>
		<Dialog.Content
			class="fixed start-1/2 top-1/2 flex max-h-[85vh] w-full -translate-x-1/2 -translate-y-1/2 flex-col overflow-y-auto rounded-[var(--radius-card,var(--radius-md,12px))] border border-[var(--color-border)] bg-[var(--color-surface,var(--background))] {padded ? 'p-5' : ''} shadow-xl {sizes[size] ?? size} data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=open]:fade-in-0 data-[state=closed]:fade-out-0 data-[state=open]:zoom-in-95 data-[state=closed]:zoom-out-95 {className}"
			style="z-index: {zIndex}"
			onInteractOutside={(e) => {
				// Autocomplete dropdowns portal to document.body to escape the
				// content's overflow-y-auto clipping, so option clicks land
				// outside this element — don't let them dismiss the dialog.
				if ((e.target as HTMLElement | null)?.closest?.('.autocomplete-dropdown')) {
					e.preventDefault();
				}
			}}
		>
			{#if !hideHeader}
				{#if header}
					{#if !title}
						<Dialog.Title class="sr-only">{ariaTitle ?? 'Dialog'}</Dialog.Title>
					{/if}
					{@render header(requestClose)}
				{:else}
					<div class="mb-4 flex items-center gap-2">
						{#if Icon}
							<Icon size={18} class="text-[var(--color-primary,var(--color-accent))]" />
						{/if}
						{#if title}
							<Dialog.Title class="text-base font-medium text-[var(--color-text,var(--foreground))]">
								{#if typeof title === 'string'}{title}{:else}{@render title()}{/if}
							</Dialog.Title>
						{:else}
							<Dialog.Title class="sr-only">{ariaTitle ?? 'Dialog'}</Dialog.Title>
						{/if}
						<Dialog.Close
							class="ms-auto grid h-7 w-7 shrink-0 place-items-center rounded-[var(--radius,var(--radius-9px,8px))] text-[var(--color-muted-foreground,var(--muted-foreground,var(--color-muted)))] transition-colors hover:bg-[var(--color-surface-subtle,var(--muted))]"
							aria-label={closeLabel}
						>
							<X size={16} />
						</Dialog.Close>
					</div>
				{/if}
			{:else}
				<Dialog.Title class="sr-only">
					{#if title && typeof title !== 'string'}{@render title()}{:else}{title ?? ariaTitle ?? 'Dialog'}{/if}
				</Dialog.Title>
			{/if}
			{#if description}
				<Dialog.Description class="mb-4 text-sm text-[var(--color-muted-foreground,var(--muted-foreground,var(--color-muted)))]">
					{#if typeof description === 'string'}{description}{:else}{@render description()}{/if}
				</Dialog.Description>
			{/if}
			{@render children?.(requestClose)}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
