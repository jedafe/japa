<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { t } from '$lib/i18n';
	import {
		analyzeComparison,
		explainComparison,
		rankComparison,
		recommendComparison,
		streamAdvisor
	} from '$lib/api/jce';
	import type { AdvisorResult, JCEItem } from '$lib/types/jce';
	import JceSelect from '$lib/components/ui/JceSelect.svelte';
	import { X, Sparkles } from 'lucide-svelte';

	interface Props {
		comparisonID: string;
		open?: boolean;
		profileID?: string;
		items?: JCEItem[];
	}

	let {
		comparisonID,
		open = $bindable(false),
		profileID,
		items = []
	}: Props = $props();

	type Mode = 'advisor' | 'analyze' | 'rank' | 'explain' | 'recommend';

	let question = $state(t('jce.advisor.default_question', 'Which is best for me?'));
	let mode = $state<Mode>('advisor');
	let loading = $state(false);
	let streaming = $state(false);
	let streamed = $state('');
	let result = $state<AdvisorResult | null>(null);
	let error = $state('');
	let selectedItemId = $state('');
	let factorKey = $state('');

	let itemOptions = $derived([
		{ value: '', label: t('jce.advisor.select_item', 'Select an item') },
		...items.map((it) => ({ value: it.id ?? '', label: it.title }))
	]);

	function reset() {
		result = null;
		streamed = '';
		error = '';
	}

	async function askStream() {
		reset();
		mode = 'advisor';
		streaming = true;
		loading = true;
		try {
			await streamAdvisor(comparisonID, question, profileID, (chunk) => {
				if (chunk.content) streamed += chunk.content;
			});
		} catch (e) {
			error = e instanceof Error ? e.message : t('jce.advisor.error', 'Advisor failed');
		} finally {
			streaming = false;
			loading = false;
		}
	}

	async function runAnalyze() {
		reset();
		mode = 'analyze';
		loading = true;
		try {
			result = await analyzeComparison(comparisonID, question, profileID);
		} catch (e) {
			error = e instanceof Error ? e.message : t('jce.advisor.error', 'Advisor failed');
		} finally {
			loading = false;
		}
	}

	async function runRank() {
		reset();
		mode = 'rank';
		loading = true;
		try {
			result = await rankComparison(comparisonID, profileID);
		} catch (e) {
			error = e instanceof Error ? e.message : t('jce.advisor.error', 'Advisor failed');
		} finally {
			loading = false;
		}
	}

	async function runRecommend() {
		reset();
		mode = 'recommend';
		loading = true;
		try {
			result = await recommendComparison(comparisonID, profileID);
		} catch (e) {
			error = e instanceof Error ? e.message : t('jce.advisor.error', 'Advisor failed');
		} finally {
			loading = false;
		}
	}

	async function runExplain() {
		if (!selectedItemId) {
			mode = 'explain';
			return;
		}
		reset();
		mode = 'explain';
		loading = true;
		try {
			result = await explainComparison(comparisonID, selectedItemId, factorKey, question, profileID);
		} catch (e) {
			error = e instanceof Error ? e.message : t('jce.advisor.error', 'Advisor failed');
		} finally {
			loading = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-40 bg-black/50" />
		<Dialog.Content
			class="fixed inset-y-0 right-0 z-50 flex w-full flex-col overflow-hidden border-l shadow-lg outline-none sm:w-[28rem]"
			style="background-color: var(--color-surface); border-color: var(--color-border)"
		>
			<Dialog.Title class="sr-only">
				{t('jce.advisor.title', 'AI Advisor')}
			</Dialog.Title>
			<Dialog.Description class="sr-only">
				{t('jce.advisor.description', 'Ask the AI advisor about this comparison')}
			</Dialog.Description>

			<div
				class="flex items-center justify-between border-b p-4"
				style="border-color: var(--color-border)"
			>
				<h2 class="flex items-center gap-2 text-base font-semibold" style="color: var(--color-text)">
					<Sparkles size={18} />
					{t('jce.advisor.title', 'AI Advisor')}
				</h2>
				<Dialog.Close
					class="grid h-8 w-8 place-items-center rounded-[var(--radius-button)] transition-colors"
					style="background-color: var(--color-bg); color: var(--color-muted)"
					aria-label={t('jce.common.close', 'Close')}
				>
					<X size={16} />
				</Dialog.Close>
			</div>

			<div class="flex-1 space-y-4 overflow-y-auto p-4">
				<div class="space-y-1">
					<label class="block space-y-1 text-sm" style="color: var(--color-text)">
						<span>{t('jce.advisor.question', 'Question')}</span>
						<textarea
							bind:value={question}
							rows={2}
							class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
							style="border-color: var(--color-border); color: var(--color-text)"
						></textarea>
					</label>
				</div>

				{#if mode === 'explain'}
					<div class="grid gap-3 sm:grid-cols-2">
						<label class="block space-y-1 text-sm" style="color: var(--color-text)">
							<span>{t('jce.advisor.item', 'Item')}</span>
							<JceSelect
								options={itemOptions}
								value={selectedItemId}
								onchange={(value) => (selectedItemId = value)}
								class="w-full"
							/>
						</label>
						<label class="block space-y-1 text-sm" style="color: var(--color-text)">
							<span>{t('jce.advisor.factor', 'Factor key')}</span>
							<input
								type="text"
								bind:value={factorKey}
								class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
								style="border-color: var(--color-border); color: var(--color-text)"
							/>
						</label>
					</div>
				{/if}

				<div class="flex flex-wrap gap-2">
					<button
						type="button"
						onclick={askStream}
						disabled={streaming || loading}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius-button)] px-3 py-1.5 text-xs font-medium disabled:opacity-50"
						style="background-color: var(--color-primary, #3b82f6); color: white"
					>
						{streaming ? t('jce.advisor.thinking', 'Thinking...') : t('jce.advisor.ask', 'Ask')}
					</button>
					<button
						type="button"
						onclick={runAnalyze}
						disabled={loading}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius-button)] border px-3 py-1.5 text-xs font-medium disabled:opacity-50"
						style="border-color: var(--color-border); background-color: var(--color-bg); color: var(--color-text)"
					>
						{t('jce.advisor.analyze', 'Analyze')}
					</button>
					<button
						type="button"
						onclick={runRank}
						disabled={loading}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius-button)] border px-3 py-1.5 text-xs font-medium disabled:opacity-50"
						style="border-color: var(--color-border); background-color: var(--color-bg); color: var(--color-text)"
					>
						{t('jce.advisor.rank', 'Rank')}
					</button>
					<button
						type="button"
						onclick={runRecommend}
						disabled={loading}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius-button)] border px-3 py-1.5 text-xs font-medium disabled:opacity-50"
						style="border-color: var(--color-border); background-color: var(--color-bg); color: var(--color-text)"
					>
						{t('jce.advisor.recommend', 'Recommend')}
					</button>
					<button
						type="button"
						onclick={runExplain}
						disabled={loading || (mode === 'explain' && !selectedItemId)}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius-button)] border px-3 py-1.5 text-xs font-medium disabled:opacity-50"
						style="border-color: var(--color-border); background-color: var(--color-bg); color: var(--color-text)"
					>
						{t('jce.advisor.explain', 'Explain')}
					</button>
				</div>

				{#if error}
					<div
						class="rounded-[var(--radius-button)] border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-400"
					>
						{error}
					</div>
				{/if}

				{#if streaming || streamed}
					<div class="space-y-1">
						<h3 class="text-sm font-medium" style="color: var(--color-muted)">
							{t('jce.advisor.stream', 'Answer')}
						</h3>
						<div
							class="rounded-[var(--radius-button)] border p-3 text-sm whitespace-pre-wrap"
							style="border-color: var(--color-border); background-color: var(--color-bg); color: var(--color-text)"
						>
							{streamed || (streaming ? t('jce.advisor.thinking', 'Thinking...') : '')}
						</div>
					</div>
				{/if}

				{#if result}
					<div class="space-y-4">
						<div
							class="rounded-[var(--radius-button)] border p-3 text-sm whitespace-pre-wrap"
							style="border-color: var(--color-border); background-color: var(--color-bg); color: var(--color-text)"
						>
							{result.summary}
						</div>

						{#if result.ranking?.length}
							<div>
								<h3 class="mb-2 text-sm font-medium" style="color: var(--color-text)">
									{t('jce.advisor.ranking', 'Ranking')}
								</h3>
								<ol class="list-decimal space-y-1 pl-4 text-sm" style="color: var(--color-text)">
									{#each result.ranking as item (item.item_id)}
										<li>
											{item.rank}. {item.item_title} — {Math.round(item.japa_score)}
											{#if !item.eligible}({t('jce.score.ineligible', 'Ineligible')}){/if}
										</li>
									{/each}
								</ol>
							</div>
						{/if}

						{#if result.next_actions?.length}
							<div>
								<h3 class="mb-2 text-sm font-medium" style="color: var(--color-text)">
									{t('jce.advisor.next_actions', 'Next actions')}
								</h3>
								<ul class="list-disc space-y-1 pl-4 text-sm" style="color: var(--color-text)">
									{#each result.next_actions as action (action.text + (action.item_id ?? ''))}
										<li>
											{#if action.action_url}
												<a
													href={action.action_url}
													target="_blank"
													class="underline"
													style="color: var(--color-text)"
												>
													{action.text}
												</a>
											{:else}
												{action.text}
											{/if}
										</li>
									{/each}
								</ul>
							</div>
						{/if}

						{#if result.citations?.length}
							<div class="text-xs" style="color: var(--color-muted)">
								<p class="font-medium">{t('jce.advisor.citations', 'Sources')}:</p>
								<ul class="list-disc space-y-0.5 pl-4">
									{#each result.citations as c (c)}
										<li>{c}</li>
									{/each}
								</ul>
							</div>
						{/if}
					</div>
				{/if}
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
