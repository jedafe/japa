<script lang="ts">
  import { Tooltip } from "bits-ui";
  import { clsx, type ClassValue } from "clsx";
  
  let {
    content,
    side = "top",
    sideOffset = 4,
    delayDuration = 200,
    class: className,
    children
  }: {
    content: string;
    side?: "top" | "right" | "bottom" | "left";
    sideOffset?: number;
    delayDuration?: number;
    class?: ClassValue;
    children?: any;
  } = $props();
</script>

<Tooltip.Provider {delayDuration}>
  <Tooltip.Root>
    <Tooltip.Trigger>
      {@render children?.()}
    </Tooltip.Trigger>
    <Tooltip.Portal>
      <Tooltip.Content
        {side}
        {sideOffset}
        class={clsx(
          "z-50 overflow-hidden rounded-lg bg-gray-900 px-3 py-1.5 text-xs text-white shadow-md animate-in fade-in-0 zoom-in-95",
          className
        )}
      >
        {content}
        <Tooltip.Arrow class="fill-gray-900" />
      </Tooltip.Content>
    </Tooltip.Portal>
  </Tooltip.Root>
</Tooltip.Provider>
