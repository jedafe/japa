<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
  import { Dialog } from "bits-ui";
  import { X } from "lucide-svelte";
  import { clsx, type ClassValue } from "clsx";
  import type { Snippet } from "svelte";

  let {
    open = $bindable(false),
    title,
    description,
    size = "default",
    class: className,
    children,
    footer,
    onclose
  }: {
    open?: boolean;
    title?: string;
    description?: string;
    size?: "sm" | "default" | "lg" | "xl";
    class?: ClassValue;
    children?: any;
    footer?: Snippet;
    onclose?: () => void;
  } = $props();
  
  const sizes = {
    sm: "max-w-sm",
    default: "max-w-lg",
    lg: "max-w-2xl",
    xl: "max-w-4xl"
  };
  
  function handleClose() {
    open = false;
    onclose?.();
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 z-50 bg-black/80 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=closed]:animate-out data-[state=closed]:fade-out-0" />
    <Dialog.Content
      data-modal-content
      class={clsx(
        "fixed start-[50%] top-[50%] z-50 flex max-h-[85dvh] w-[calc(100%-2rem)] translate-x-[-50%] translate-y-[-50%] flex-col gap-4 overflow-hidden rounded-xl border p-4 shadow-xl duration-200 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 data-[state=open]:slide-in-from-left-1/2 data-[state=open]:slide-in-from-top-[48%] data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=closed]:slide-out-to-left-1/2 data-[state=closed]:slide-out-to-top-[48%] sm:p-6",
        sizes[size],
        className
      )}
      style="border-color: var(--color-border); background-color: var(--color-surface)"
      onInteractOutside={(e) => {
        // Autocomplete dropdowns are portaled to <body>, outside this
        // Dialog.Content — don't let an option click dismiss the dialog.
        if ((e.target as HTMLElement | null)?.closest?.('.autocomplete-dropdown')) {
          e.preventDefault();
        }
      }}
    >
      {#if title || description}
        <div class="flex shrink-0 flex-col space-y-1.5 text-center sm:text-start">
          {#if title}
            <Dialog.Title class="text-lg font-semibold leading-none tracking-tight" style="color: var(--color-foreground)">
              {title}
            </Dialog.Title>
          {/if}
          {#if description}
            <Dialog.Description class="text-sm" style="color: var(--color-muted)">
              {description}
            </Dialog.Description>
          {/if}
        </div>
      {/if}
      
      <div class="min-h-0 overflow-y-auto overscroll-contain">
        {@render children?.()}
      </div>

      {#if footer}
        <div class="flex shrink-0 items-center justify-end gap-2 border-t pt-4" style="border-color: var(--color-border)">
          {@render footer()}
        </div>
      {/if}

      <Dialog.Close
        class="absolute end-4 top-4 rounded-lg p-1 opacity-70 transition-opacity hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:pointer-events-none"
        style="color: var(--color-muted)"
        onclick={handleClose}
      >
        <X class="h-4 w-4" />
        <span class="sr-only">{t('lib.components.ui.modal.modal.svelte.close')}</span>
      </Dialog.Close>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
