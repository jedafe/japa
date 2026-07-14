<script lang="ts">
  import { clsx, type ClassValue } from "clsx";
  
  let {
    class: className,
    children,
    ...restProps
  }: {
    class?: ClassValue;
    children?: any;
    [key: string]: any;
  } = $props();
</script>

<div
  class={clsx(
    "rounded-xl border border-gray-200 bg-white text-gray-950 shadow-sm",
    className
  )}
  {...restProps}
>
  {@render children?.()}
</div>
