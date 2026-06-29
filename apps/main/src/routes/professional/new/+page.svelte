<script lang="ts">
	import { goto } from '$app/navigation';
	import { ArrowLeft, Loader2, Plus, X } from 'lucide-svelte';
	import { api, getToken } from '$lib/api';

	let name = $state('');
	let headline = $state('');
	let skills = $state<string[]>([]);
	let skillInput = $state('');
	let isPublic = $state(true);
	let error = $state('');
	let loading = $state(false);

	function addSkill() {
		const trimmed = skillInput.trim();
		if (trimmed && !skills.includes(trimmed)) {
			skills = [...skills, trimmed];
		}
		skillInput = '';
	}

	function removeSkill(skill: string) {
		skills = skills.filter((s) => s !== skill);
	}

	function handleSkillKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			addSkill();
		}
	}

	function authHeaders(): Record<string, string> {
		const h: Record<string, string> = { 'Content-Type': 'application/json' };
		const token = getToken();
		if (token) h['Authorization'] = `Bearer ${token}`;
		return h;
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';
		loading = true;

		try {
			const res = await fetch('/api/v1/professional-profiles', {
				method: 'POST',
				headers: authHeaders(),
				body: JSON.stringify({ name, headline, skills, is_public: isPublic })
			});

			const body = await res.json();
			if (!res.ok || body.error) {
				error = body.error?.message || 'Failed to create profile.';
				return;
			}

			goto(`/professional/${body.data.slug}`);
		} catch {
			error = 'Network error. Please try again.';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Create Professional Profile | JapaConnect</title>
	<meta name="description" content="Create your professional profile on JapaConnect." />
</svelte:head>

<main class="mx-auto max-w-lg px-4 py-8">
	<a
		href="/professional"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to profiles
	</a>

	<div class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
		<h1 class="text-xl font-bold" style="color: var(--color-foreground)">Create professional profile</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">
			Showcase your skills and relocation journey.
		</p>

		{#if error}
			<div
				class="mt-4 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
				{error}
			</div>
		{/if}

		<form onsubmit={handleSubmit} class="mt-6 space-y-4">
			<div>
				<label for="name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					Name
				</label>
				<input
					id="name"
					type="text"
					required
					maxlength="100"
					bind:value={name}
					placeholder="Your professional name"
					class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
					style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
				/>
			</div>

			<div>
				<label for="headline" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					Headline
				</label>
				<input
					id="headline"
					type="text"
					required
					maxlength="200"
					bind:value={headline}
					placeholder="e.g. Senior Software Engineer | Open to relocation"
					class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
					style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
				/>
			</div>

			<div>
				<label for="skills" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					Skills
				</label>
				<div class="flex gap-2">
					<input
						id="skills"
						type="text"
						bind:value={skillInput}
						onkeydown={handleSkillKeydown}
						placeholder="Type a skill and press Enter"
						class="flex-1 rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
					/>
					<button
						type="button"
						onclick={addSkill}
						class="rounded-lg px-3 py-2 text-sm font-medium transition-colors"
						style="background-color: var(--color-background); color: var(--color-foreground); border: 1px solid var(--color-border)"
					>
						Add
					</button>
				</div>
				{#if skills.length > 0}
					<div class="mt-2 flex flex-wrap gap-1.5">
						{#each skills as skill}
							<span
								class="inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium"
								style="background-color: var(--color-primary); color: white"
							>
								{skill}
								<button type="button" onclick={() => removeSkill(skill)} class="hover:opacity-70">
									<X class="h-3 w-3" />
								</button>
							</span>
						{/each}
					</div>
				{/if}
			</div>

			<div class="flex items-center gap-2">
				<input
					id="is_public"
					type="checkbox"
					bind:checked={isPublic}
					class="h-4 w-4 rounded"
					style="accent-color: var(--color-primary)"
				/>
				<label for="is_public" class="text-sm" style="color: var(--color-foreground)">
					Make profile public
				</label>
			</div>

			<button
				type="submit"
				disabled={loading || !name.trim() || !headline.trim()}
				class="flex w-full items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				style="background-color: var(--color-primary)"
			>
				{#if loading}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<Plus class="h-4 w-4" />
				{/if}
				Create profile
			</button>
		</form>
	</div>
</main>
