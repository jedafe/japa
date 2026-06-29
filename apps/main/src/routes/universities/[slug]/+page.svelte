<script lang="ts">
	import type { PageData } from './$types';
	import { MetadataHead, JsonLd } from '$lib/seo';
	import { UniversityHeader, UniversityTabs, UniversitySidebar } from '$lib/universities';
	import { ErrorState } from '$lib/components/ui';
	import { UserPlus, UserMinus } from 'lucide-svelte';
	import { getToken } from '$lib/api';

	let { data }: { data: PageData } = $props();

	let activeTab = $state('overview');
	let isFollowing = $state(false);
	let isLoading = $state(false);
	let errorMessage = $state('');

	const universitySchema = $derived({
		'@context': 'https://schema.org',
		'@type': 'EducationalOrganization',
		name: data.university.name,
		description: data.university.description,
		address: {
			'@type': 'PostalAddress',
			addressCountry: data.university.country
		},
		...(data.reviewAggregate && {
			aggregateRating: {
				'@type': 'AggregateRating',
				ratingValue: data.reviewAggregate.average_rating,
				reviewCount: data.reviewAggregate.review_count
			}
		})
	});

	async function toggleFollow() {
		if (isLoading) return;
		isLoading = true;
		errorMessage = '';
		try {
			const headers: Record<string, string> = { 'Content-Type': 'application/json' };
			const token = getToken();
			if (token) headers['Authorization'] = `Bearer ${token}`;
			const response = await fetch('/api/v1/follows', {
				method: 'POST',
				headers,
				body: JSON.stringify({
					entity_type: 'space',
					entity_id: data.university.id
				})
			});
			if (response.ok) {
				isFollowing = !isFollowing;
			} else {
				errorMessage = 'Failed to update follow status. Please try again.';
			}
		} catch {
			errorMessage = 'Failed to update follow status. Please try again.';
		} finally {
			isLoading = false;
		}
	}
</script>

<MetadataHead
	metadata={{
		title: `${data.university.name} | JapaConnect`,
		description: data.university.description || `Learn about ${data.university.name} on JapaConnect`,
		og_image: data.university.logo_url
	}}
/>

<JsonLd schema={universitySchema} />

<div class="mx-auto max-w-6xl px-4 py-8">
	<UniversityHeader
		university={{
			...data.university,
			rating: data.reviewAggregate?.average_rating,
			reviewCount: data.reviewAggregate?.review_count
		}}
	/>

	<div class="mt-6 flex flex-wrap items-center gap-3">
		<button
			onclick={toggleFollow}
			disabled={isLoading}
			class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-colors"
			style="background-color: {isFollowing ? 'var(--color-muted)' : 'var(--color-primary)'}; color: white"
		>
			{#if isFollowing}
				<UserMinus class="h-4 w-4" />
				Unfollow
			{:else}
				<UserPlus class="h-4 w-4" />
				Follow
			{/if}
		</button>
		{#if errorMessage}
			<span class="text-sm" style="color: var(--color-danger)">{errorMessage}</span>
		{/if}
	</div>

	<div class="mt-6">
		<UniversityTabs bind:activeTab />
	</div>

	<div class="mt-6 grid gap-6 lg:grid-cols-3">
		<div class="lg:col-span-2">
			{#if activeTab === 'overview'}
				<div class="prose max-w-none" style="color: var(--color-foreground)">
					{#if data.university.description}
						<p>{data.university.description}</p>
					{:else}
						<p style="color: var(--color-muted)">
							No description available for this university.
						</p>
					{/if}
				</div>
			{:else if activeTab === 'reviews'}
				<div class="text-center py-8" style="color: var(--color-muted)">
					<p>Reviews section coming soon.</p>
				</div>
			{:else if activeTab === 'compare'}
				<div class="text-center py-8" style="color: var(--color-muted)">
					<p>Select universities to compare.</p>
				</div>
			{:else}
				<div class="text-center py-8" style="color: var(--color-muted)">
					<p>This section is coming soon.</p>
				</div>
			{/if}
		</div>

		<div class="lg:col-span-1">
			<UniversitySidebar fields={data.fields} />
		</div>
	</div>
</div>
