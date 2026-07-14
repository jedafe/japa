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
  class={clsx("p-6 pt-0", className)}
  {...restProps}
>
  {@render children?.()}
</div>
