<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Certificate } from '$lib/api';
	import { Award, Download, ExternalLink, GraduationCap, ArrowRight } from 'lucide-svelte';

	let certificates = $state<Certificate[]>([]);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		try {
			certificates = await api.getCertificates();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load certificates';
		} finally {
			loading = false;
		}
	});

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString('en-US', {
			year: 'numeric',
			month: 'long',
			day: 'numeric'
		});
	}
</script>

<svelte:head>
	<title>My Certificates | JapaConnect Learn</title>
</svelte:head>

<main class="mx-auto max-w-4xl px-4 py-8 sm:px-6">
	<div class="flex items-center gap-3">
		<div class="rounded-lg p-2" style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent)">
			<Award class="h-6 w-6" style="color: var(--color-primary)" />
		</div>
		<div>
			<h1 class="text-2xl font-bold tracking-tight" style="color: var(--color-foreground)">My Certificates</h1>
			<p class="text-sm" style="color: var(--color-muted)">
				Certificates earned by completing learning tracks
			</p>
		</div>
	</div>

	{#if loading}
		<div class="mt-8 space-y-4">
			{#each Array(2) as _}
				<div class="animate-pulse rounded-lg border p-6" style="border-color: var(--color-border)">
					<div class="flex items-start gap-4">
						<div class="h-12 w-12 rounded-lg" style="background-color: var(--color-surface)"></div>
						<div class="flex-1 space-y-2">
							<div class="h-4 w-1/2 rounded" style="background-color: var(--color-surface)"></div>
							<div class="h-3 w-1/4 rounded" style="background-color: var(--color-surface)"></div>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{:else if error}
		<div class="mt-8 rounded-lg border px-6 py-12 text-center" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 5%, transparent)">
			<p class="text-sm" style="color: var(--color-danger)">{error}</p>
			<button
				onclick={() => window.location.reload()}
				class="mt-4 rounded-md px-4 py-2 text-sm font-medium text-white"
				style="background-color: var(--color-primary)"
			>
				Try again
			</button>
		</div>
	{:else if certificates.length === 0}
		<div class="mt-8 rounded-lg border px-6 py-16 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<GraduationCap class="mx-auto h-16 w-16" style="color: var(--color-muted)" />
			<h2 class="mt-4 text-lg font-semibold" style="color: var(--color-foreground)">No certificates yet</h2>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				Complete a learning track to earn your first certificate!
			</p>
			<a
				href="/"
				class="mt-6 inline-flex items-center gap-2 rounded-lg px-5 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
				style="background-color: var(--color-primary)"
			>
				Browse tracks
				<ArrowRight class="h-4 w-4" />
			</a>
		</div>
	{:else}
		<div class="mt-8 space-y-4">
			{#each certificates as cert (cert.id)}
				<article class="rounded-lg border p-6 transition-all hover:shadow-md" style="border-color: var(--color-border)">
					<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
						<div class="flex items-start gap-4">
							<div class="rounded-lg p-2.5" style="background-color: color-mix(in srgb, var(--color-success) 10%, transparent)">
								<Award class="h-6 w-6" style="color: var(--color-success)" />
							</div>
							<div>
								<h3 class="font-semibold" style="color: var(--color-foreground)">{cert.track_title}</h3>
								<p class="mt-1 text-sm" style="color: var(--color-muted)">
									Issued {formatDate(cert.issued_at)}
								</p>
							</div>
						</div>

						<div class="flex items-center gap-2">
							{#if cert.certificate_url}
								<a
									href={cert.certificate_url}
									target="_blank"
									rel="noopener noreferrer"
									class="flex items-center gap-1.5 rounded-lg border px-3 py-2 text-sm font-medium transition-colors hover:bg-[var(--color-surface)]"
									style="border-color: var(--color-border); color: var(--color-foreground)"
								>
									<ExternalLink class="h-4 w-4" />
									View
								</a>
								<a
									href={cert.certificate_url}
									download
									class="flex items-center gap-1.5 rounded-lg px-3 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
									style="background-color: var(--color-primary)"
								>
									<Download class="h-4 w-4" />
									Download
								</a>
							{:else}
								<span class="text-sm" style="color: var(--color-muted)">Certificate pending</span>
							{/if}
						</div>
					</div>
				</article>
			{/each}
		</div>
	{/if}
</main>
