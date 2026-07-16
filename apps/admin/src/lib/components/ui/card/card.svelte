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
  class={clsx("rounded-xl border shadow-sm", className)}
  style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
  {...restProps}
>
  {@render children?.()}
</div>
