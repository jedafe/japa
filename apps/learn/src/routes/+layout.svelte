<script lang="ts">
	import '../app.css';
	import { t } from '$lib/stores/i18n.svelte';
	import { GraduationCap, BookOpen, Compass, User } from 'lucide-svelte';

	let { children } = $props();

	const navLinks = [
		{ href: '/', labelKey: 'nav.home', icon: Compass },
		{ href: '/#tracks', labelKey: 'nav.tracks', icon: GraduationCap },
		{ href: '/#courses', labelKey: 'nav.courses', icon: BookOpen }
	];
</script>

<div class="flex min-h-screen flex-col">
	<header class="border-b" style="border-color: var(--color-border)">
		<div class="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
			<a href="/" class="flex items-center gap-2 font-bold tracking-tight">
				<GraduationCap class="h-6 w-6" style="color: var(--color-primary)" />
				{$t('learn.brand')}
			</a>

			<nav class="flex items-center gap-6">
				{#each navLinks as link}
					<a
						href={link.href}
						class="flex items-center gap-1.5 text-sm font-medium transition-colors hover:opacity-80"
						style="color: var(--color-muted)"
					>
						<svelte:component this={link.icon} class="h-4 w-4" />
						{$t(link.labelKey)}
					</a>
				{/each}
				<a
					href="/"
					class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium text-white"
					style="background-color: var(--color-primary)"
				>
					<User class="h-4 w-4" />
					{$t('nav.login')}
				</a>
			</nav>
		</div>
	</header>

	<main class="flex-1">
		{@render children()}
	</main>

	<footer class="border-t py-6 text-center text-sm" style="border-color: var(--color-border); color: var(--color-muted)">
		{$t('footer.copyright')}
	</footer>
</div>
