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
  class={clsx("flex items-center p-4 pt-0 sm:p-6 sm:pt-0", className)}
  {...restProps}
>
  {@render children?.()}
</div>
