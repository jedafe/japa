<script lang="ts">
  import { Dialog } from "bits-ui";
  import { X } from "lucide-svelte";
  import { clsx, type ClassValue } from "clsx";
  
  let {
    open = $bindable(false),
    title,
    description,
    size = "default",
    class: className,
    children,
    onclose
  }: {
    open?: boolean;
    title?: string;
    description?: string;
    size?: "sm" | "default" | "lg" | "xl";
    class?: ClassValue;
    children?: any;
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
      class={clsx(
        "fixed left-[50%] top-[50%] z-50 grid w-full translate-x-[-50%] translate-y-[-50%] gap-4 border border-gray-200 bg-white p-6 shadow-xl duration-200 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 data-[state=open]:slide-in-from-left-1/2 data-[state=open]:slide-in-from-top-[48%] data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=closed]:slide-out-to-left-1/2 data-[state=closed]:slide-out-to-top-[48%] sm:rounded-xl",
        sizes[size],
        className
      )}
    >
      {#if title || description}
        <div class="flex flex-col space-y-1.5 text-center sm:text-left">
          {#if title}
            <Dialog.Title class="text-lg font-semibold leading-none tracking-tight text-gray-900">
              {title}
            </Dialog.Title>
          {/if}
          {#if description}
            <Dialog.Description class="text-sm text-gray-500">
              {description}
            </Dialog.Description>
          {/if}
        </div>
      {/if}
      
      {@render children?.()}
      
      <Dialog.Close
        class="absolute right-4 top-4 rounded-lg p-1 opacity-70 ring-offset-white transition-opacity hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-gray-400 focus:ring-offset-2 disabled:pointer-events-none data-[state=open]:bg-gray-100"
        onclick={handleClose}
      >
        <X class="h-4 w-4" />
        <span class="sr-only">Close</span>
      </Dialog.Close>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
