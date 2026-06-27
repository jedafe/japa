<script lang="ts">
	import { BookOpen, Code, Users, Shield, GraduationCap, Building } from 'lucide-svelte';

	const paths = [
		{ slug: 'users', label: 'User Guide', description: 'Getting started, profiles, content creation', icon: BookOpen },
		{ slug: 'premium', label: 'Premium', description: 'Membership benefits and features', icon: GraduationCap },
		{ slug: 'organizations', label: 'Organizations', description: 'Team management and collaboration', icon: Building },
		{ slug: 'moderators', label: 'Moderators', description: 'Moderation tools and guidelines', icon: Shield },
		{ slug: 'api', label: 'API Reference', description: 'Developer documentation and endpoints', icon: Code },
		{ slug: 'admin', label: 'Admin', description: 'Platform administration and configuration', icon: Users },
	];
</script>

<svelte:head>
	<title>JapaConnect Docs</title>
</svelte:head>

<div class="mx-auto max-w-4xl">
	<h1 class="text-4xl font-bold tracking-tight">JapaConnect Documentation</h1>
	<p class="mt-3 text-lg" style="color: var(--color-muted)">
		Guides, tutorials, and API reference for the JapaConnect platform.
	</p>

	<div class="mt-10 grid gap-4 sm:grid-cols-2">
		{#each paths as path}
			<a
				href="/{path.slug}"
				class="group rounded-lg border p-6 transition-all hover:shadow-md"
				style="border-color: var(--color-border)"
			>
				<div class="flex items-start gap-3">
					<svelte:component this={path.icon} class="mt-0.5 h-5 w-5" style="color: var(--color-primary)" />
					<div>
						<h2 class="font-semibold group-hover:text-blue-600">{path.label}</h2>
						<p class="mt-1 text-sm" style="color: var(--color-muted)">{path.description}</p>
					</div>
				</div>
			</a>
		{/each}
	</div>
</div>
