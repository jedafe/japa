<script lang="ts">
  import { clsx, type ClassValue } from "clsx";
  
  let {
    class: className,
    variant = "text",
    ...restProps
  }: {
    class?: ClassValue;
    variant?: "text" | "circular" | "rectangular";
    [key: string]: any;
  } = $props();
</script>

<div
  class={clsx(
    "animate-pulse",
    variant === "text" && "h-4 w-full rounded",
    variant === "circular" && "h-10 w-10 rounded-full",
    variant === "rectangular" && "h-20 w-full rounded-lg",
    className
  )}
  style="background-color: var(--color-muted); opacity: 0.3"
  {...restProps}
></div>
