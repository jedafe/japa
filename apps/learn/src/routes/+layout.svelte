<script lang="ts">
	import '../app.css';
	import { GraduationCap, BookOpen, Compass, User } from 'lucide-svelte';

	let { children } = $props();

	const navLinks = [
		{ href: '/', label: 'Home', icon: Compass },
		{ href: '/#tracks', label: 'Tracks', icon: GraduationCap },
		{ href: '/#courses', label: 'Courses', icon: BookOpen }
	];
</script>

<div class="flex min-h-screen flex-col">
	<header class="border-b" style="border-color: var(--color-border)">
		<div class="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
			<a href="/" class="flex items-center gap-2 font-bold tracking-tight">
				<GraduationCap class="h-6 w-6" style="color: var(--color-primary)" />
				JapaConnect Learn
			</a>

			<nav class="flex items-center gap-6">
				{#each navLinks as link}
					<a
						href={link.href}
						class="flex items-center gap-1.5 text-sm font-medium transition-colors hover:opacity-80"
						style="color: var(--color-muted)"
					>
						<svelte:component this={link.icon} class="h-4 w-4" />
						{link.label}
					</a>
				{/each}
				<a
					href="/"
					class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium text-white"
					style="background-color: var(--color-primary)"
				>
					<User class="h-4 w-4" />
					Sign in
				</a>
			</nav>
		</div>
	</header>

	<main class="flex-1">
		{@render children()}
	</main>

	<footer class="border-t py-6 text-center text-sm" style="border-color: var(--color-border); color: var(--color-muted)">
		JapaConnect Learn &copy; 2026
	</footer>
</div>
