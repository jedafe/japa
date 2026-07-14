<script lang="ts">
  import { clsx, type ClassValue } from "clsx";
  
  let {
    variant = "default",
    class: className,
    children,
    ...restProps
  }: {
    variant?: "default" | "secondary" | "destructive" | "outline" | "success" | "warning" | "info";
    class?: ClassValue;
    children?: any;
    [key: string]: any;
  } = $props();
  
  const variants = {
    default: "bg-primary-100 text-primary-800 border-primary-200",
    secondary: "bg-gray-100 text-gray-800 border-gray-200",
    destructive: "bg-danger-100 text-danger-800 border-danger-200",
    outline: "text-gray-950 border-gray-300",
    success: "bg-success-100 text-success-800 border-success-200",
    warning: "bg-warning-100 text-warning-800 border-warning-200",
    info: "bg-info-100 text-info-800 border-info-200"
  };
</script>

<div
  class={clsx(
    "inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2",
    variants[variant],
    className
  )}
  {...restProps}
>
  {@render children?.()}
</div>
