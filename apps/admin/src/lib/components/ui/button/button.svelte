<script lang="ts">
  import { Button } from "bits-ui";
  import { clsx, type ClassValue } from "clsx";
  
  let {
    variant = "default",
    size = "default",
    class: className,
    children,
    ...restProps
  }: {
    variant?: "default" | "destructive" | "outline" | "secondary" | "ghost" | "link";
    size?: "default" | "sm" | "lg" | "icon";
    class?: ClassValue;
    children?: any;
    [key: string]: any;
  } = $props();
  
  const variants = {
    default: "bg-primary text-white hover:bg-primary shadow-sm",
    destructive: "bg-danger-600 text-white hover:bg-danger-700 shadow-sm",
    outline: "border border bg-surface hover:bg-surface text-foreground-alt",
    secondary: "bg-surface text-foreground hover:bg-surface",
    ghost: "hover:bg-surface text-foreground-alt",
    link: "text-primary underline-offset-4 hover:underline"
  };
  
  const sizes = {
    default: "h-10 px-4 py-2",
    sm: "h-9 rounded-lg px-3 text-sm",
    lg: "h-11 rounded-lg px-8",
    icon: "h-10 w-10"
  };
</script>

<Button.Root
  class={clsx(
    "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-lg text-sm font-medium transition-colors focus-ring disabled:pointer-events-none disabled:opacity-50",
    variants[variant],
    sizes[size],
    className
  )}
  {...restProps}
>
  {@render children?.()}
</Button.Root>
