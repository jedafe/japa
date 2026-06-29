<script lang="ts">
	import { BookOpen, Clock, Eye, MessageCircle, Rss, Star } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';

	let { data } = $props();

	const featured = $derived(data.featured ?? []);
	const posts = $derived(data.posts ?? []);
	const series = $derived(data.series ?? []);

	function formatDate(dateStr: string): string {
		if (!dateStr) return '';
		const d = new Date(dateStr);
		return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
	}
</script>

<svelte:head>
	<title>Blog | JapaConnect</title>
	<meta name="description" content="Stories, guides, and insights from the JapaConnect community." />
	<link rel="alternate" type="application/rss+xml" title="JapaConnect Blog RSS" href="/api/v1/blog/feed.xml" />
</svelte:head>

<main class="mx-auto max-w-6xl px-4 py-8">
	<!-- Header -->
	<header class="mb-10 flex items-start justify-between">
		<div>
			<h1 class="text-3xl font-bold tracking-tight sm:text-4xl" style="color: var(--color-foreground)">
				Blog
			</h1>
			<p class="mt-2 text-base" style="color: var(--color-muted)">
				Stories, guides, and insights from the community.
			</p>
		</div>
		<a
			href="/api/v1/blog/feed.xml"
			class="flex items-center gap-1.5 rounded-lg border px-3 py-2 text-sm font-medium transition-colors hover:opacity-80"
			style="border-color: var(--color-border); color: var(--color-muted)"
			target="_blank"
			rel="noopener noreferrer"
		>
			<Rss class="h-4 w-4" />
			RSS
		</a>
	</header>

	<div class="grid gap-8 lg:grid-cols-[1fr_280px]">
		<!-- Main Content -->
		<div>
			<!-- Featured Posts -->
			{#if featured.length > 0}
				<section class="mb-10">
					<h2 class="mb-4 flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
						<Star class="h-5 w-5" style="color: var(--color-primary)" />
						Featured
					</h2>
					<div class="grid gap-4 sm:grid-cols-2">
						{#each featured as feat (feat.id)}
							<a
								href="/blog/{feat.content_id}"
								class="group rounded-xl border p-5 transition-all hover:shadow-md"
								style="border-color: var(--color-border); background: var(--color-surface)"
							>
								<div class="flex items-center gap-2 text-xs" style="color: var(--color-primary)">
									<Star class="h-3.5 w-3.5" />
									Featured
								</div>
								<p class="mt-2 text-sm font-medium line-clamp-2" style="color: var(--color-foreground)">
									Post #{feat.featured_order}
								</p>
							</a>
						{/each}
					</div>
				</section>
			{/if}

			<!-- Recent Posts -->
			<section>
				<h2 class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">Recent Posts</h2>
				{#if posts.length === 0}
					<EmptyState
						icon={BookOpen}
						title="No posts yet"
						message="Be the first to share a story with the community."
					/>
				{:else}
					<div class="space-y-4">
						{#each posts as post (post.id)}
							<a
								href="/blog/{post.slug}"
								class="group block rounded-xl border p-5 transition-all hover:shadow-md"
								style="border-color: var(--color-border); background: var(--color-surface)"
							>
								<h3
									class="text-base font-semibold group-hover:underline sm:text-lg"
									style="color: var(--color-foreground)"
								>
									{post.title}
								</h3>
								<div class="mt-3 flex flex-wrap items-center gap-4 text-xs" style="color: var(--color-muted)">
									{#if post.published_at}
										<span>{formatDate(post.published_at)}</span>
									{:else if post.created_at}
										<span>{formatDate(post.created_at)}</span>
									{/if}
									{#if post.read_time_minutes > 0}
										<span class="flex items-center gap-1">
											<Clock class="h-3.5 w-3.5" />
											{post.read_time_minutes} min read
										</span>
									{/if}
									{#if post.view_count > 0}
										<span class="flex items-center gap-1">
											<Eye class="h-3.5 w-3.5" />
											{post.view_count}
										</span>
									{/if}
									{#if post.comment_count > 0}
										<span class="flex items-center gap-1">
											<MessageCircle class="h-3.5 w-3.5" />
											{post.comment_count}
										</span>
									{/if}
								</div>
							</a>
						{/each}
					</div>
				{/if}
			</section>
		</div>

		<!-- Sidebar -->
		<aside class="space-y-6">
			<!-- Series -->
			{#if series.length > 0}
				<div class="rounded-xl border p-5" style="border-color: var(--color-border); background: var(--color-surface)">
					<h3 class="mb-3 text-sm font-semibold uppercase tracking-wide" style="color: var(--color-muted)">
						Blog Series
					</h3>
					<ul class="space-y-3">
						{#each series as s (s.id)}
							<li>
								<a
									href="/blog/series/{s.slug}"
									class="block rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:opacity-80"
									style="color: var(--color-foreground)"
								>
									{s.title}
								</a>
								{#if s.description}
									<p class="mt-0.5 px-3 text-xs line-clamp-2" style="color: var(--color-muted)">
										{s.description}
									</p>
								{/if}
							</li>
						{/each}
					</ul>
					<a
						href="/blog/series"
						class="mt-4 block text-xs font-medium hover:underline"
						style="color: var(--color-primary)"
					>
						View all series &rarr;
					</a>
				</div>
			{/if}

			<!-- Subscribe -->
			<div class="rounded-xl border p-5" style="border-color: var(--color-border); background: var(--color-surface)">
				<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">Stay updated</h3>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">
					Subscribe to the RSS feed for new posts.
				</p>
				<a
					href="/api/v1/blog/feed.xml"
					class="mt-3 inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium text-white transition-opacity hover:opacity-90"
					style="background-color: var(--color-primary)"
					target="_blank"
					rel="noopener noreferrer"
				>
					<Rss class="h-3.5 w-3.5" />
					Subscribe via RSS
				</a>
			</div>
		</aside>
	</div>
</main>
