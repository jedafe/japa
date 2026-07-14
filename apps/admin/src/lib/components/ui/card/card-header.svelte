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
  class={clsx("flex flex-col space-y-1.5 p-6", className)}
  {...restProps}
>
  {@render children?.()}
</div>
