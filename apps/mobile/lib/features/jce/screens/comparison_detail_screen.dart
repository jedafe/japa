import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/jce/providers/jce_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class JceComparisonDetailScreen extends ConsumerStatefulWidget {
  final String id;

  const JceComparisonDetailScreen({super.key, required this.id});

  @override
  ConsumerState<JceComparisonDetailScreen> createState() => _JceComparisonDetailScreenState();
}

class _JceComparisonDetailScreenState extends ConsumerState<JceComparisonDetailScreen> {
  final _advisorController = TextEditingController();
  Timer? _pollTimer;
  int _pollCount = 0;

  @override
  void dispose() {
    _advisorController.dispose();
    _pollTimer?.cancel();
    super.dispose();
  }

  void _startPolling() {
    _pollTimer?.cancel();
    _pollCount = 0;
    _pollTimer = Timer.periodic(const Duration(milliseconds: 1500), (_) async {
      if (!mounted) return;
      if (_pollCount >= 30) {
        _pollTimer?.cancel();
        return;
      }
      _pollCount++;
      ref.invalidate(jceIngestionJobsProvider(widget.id));
      ref.invalidate(jceFreshnessProvider(widget.id));
    });
  }

  void _score(String comparisonId, String? profileId, String smartMode) {
    ref.read(scoreComparisonProvider(comparisonId).notifier).score(
          profileId: profileId,
          smartMode: smartMode,
          force: true,
        );
  }

  void _refresh(String comparisonId) {
    ref.read(refreshComparisonProvider(comparisonId).notifier).refresh();
    _startPolling();
  }

  void _askAdvisor(String comparisonId, String? profileId) {
    final question = _advisorController.text.trim();
    if (question.isEmpty) return;
    _advisorController.clear();
    ref.read(jceAdvisorArgsProvider(comparisonId).notifier).setArgs(
          JceAdvisorArgs(
            comparisonId: comparisonId,
            question: question,
            profileId: profileId,
          ),
        );
  }

  @override
  Widget build(BuildContext context) {
    final t = ref.watch(tProvider);
    final detailAsync = ref.watch(jceComparisonDetailProvider(widget.id));

    return Scaffold(
      appBar: AppBar(
        title: Text(detailAsync.value?.content.title ?? t('jce.comparison.title')),
        actions: [
          IconButton(
            icon: const Icon(Icons.person_outline),
            tooltip: t('jce.profile.title'),
            onPressed: () => _showProfileDrawer(context),
          ),
        ],
      ),
      body: detailAsync.when(
        data: (detail) {
          if (detail == null) {
            return EmptyState(message: t('jce.comparison.not_found'));
          }
          return _JceComparisonDetailBody(
            id: widget.id,
            detail: detail,
            onScore: _score,
            onRefresh: _refresh,
            onAskAdvisor: _askAdvisor,
            advisorController: _advisorController,
          );
        },
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(jceComparisonDetailProvider(widget.id)),
        ),
      ),
    );
  }

  void _showProfileDrawer(BuildContext context) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (ctx) => _ProfileDrawer(
        comparisonId: widget.id,
        onManage: () {
          Navigator.of(ctx).pop();
          context.push('/comparisons/profiles');
        },
      ),
    );
  }
}

class _JceComparisonDetailBody extends ConsumerWidget {
  final String id;
  final JceComparisonDetail detail;
  final void Function(String, String?, String) onScore;
  final void Function(String) onRefresh;
  final void Function(String, String?) onAskAdvisor;
  final TextEditingController advisorController;

  const _JceComparisonDetailBody({
    required this.id,
    required this.detail,
    required this.onScore,
    required this.onRefresh,
    required this.onAskAdvisor,
    required this.advisorController,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final matrixAsync = ref.watch(jceMatrixProvider(id));
    final capsAsync = ref.watch(jceCapabilitiesProvider);
    final caps = capsAsync.value;

    final canViewScore = caps?.canViewScore ?? false;
    final canRecalculateScore = caps?.canRecalculateScore ?? false;
    final canViewRecommendations = caps?.canViewRecommendations ?? false;
    final canViewFreshness = caps?.canViewFreshness ?? false;
    final canRefresh = caps?.canRefresh ?? false;
    final canViewIngestionJobs = caps?.canViewIngestionJobs ?? false;
    final canUseAdvisor = caps?.canUseAdvisor ?? false;
    final canUseProfiles = caps?.canUseProfiles ?? false;

    final scoreResultAsync = canViewScore
        ? ref.watch(scoreComparisonProvider(id))
        : const AsyncData<ScoreResult?>(null);
    final refreshJobAsync = canRefresh
        ? ref.watch(refreshComparisonProvider(id))
        : const AsyncData<IngestionJob?>(null);
    final freshnessAsync = canViewFreshness
        ? ref.watch(jceFreshnessProvider(id))
        : const AsyncData<FreshnessSummary?>(null);
    final jobsAsync = canViewIngestionJobs
        ? ref.watch(jceIngestionJobsProvider(id))
        : const AsyncData<List<IngestionJob>>([]);
    final recommendationsAsync = canViewRecommendations
        ? ref.watch(jceRecommendationsProvider(id))
        : const AsyncData<List<ComparisonRecommendation>>([]);
    final advisorAsync = canUseAdvisor
        ? ref.watch(jceAdvisorStreamProvider(id))
        : const AsyncData<List<AdvisorStreamChunk>>([]);
    final selectedProfileId = ref.watch(selectedProfileIdProvider);

    final resolvedItems = detail.resolvedItems;
    final matrix = matrixAsync.value ?? [];
    final typeSlug = detail.typeSlug ?? '';
    final advisorKey = GlobalKey();

    return SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            detail.content.title,
            style: JcTypography.heading,
          ),
          const SizedBox(height: 8),
          Text(
            '${detail.metadata.depth} · ${detail.content.status} · ${detail.content.visibility}',
            style: JcTypography.caption,
          ),
          if (detail.content.bodyHtml != null || detail.content.body != null) ...[
            const SizedBox(height: 16),
            Text(
              detail.content.body ?? stripHtml(detail.content.bodyHtml),
              style: JcTypography.body,
            ),
          ],
          const SizedBox(height: 24),
          Text(t('jce.comparison.items_heading'), style: JcTypography.heading),
          const SizedBox(height: 8),
          ...resolvedItems.map((item) => _ItemCard(item: item)),
          const SizedBox(height: 24),
          if (canUseProfiles || canRecalculateScore)
            _ScoreControls(
              comparisonId: id,
              typeSlug: typeSlug,
              canUseProfiles: canUseProfiles,
              canRecalculateScore: canRecalculateScore,
              onScore: onScore,
            ),
          if (canViewScore) ...[
            const SizedBox(height: 16),
            _ScorePanel(scoreAsync: scoreResultAsync),
          ],
          const SizedBox(height: 24),
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Text(t('jce.comparison.matrix_heading'), style: JcTypography.heading),
              Row(
                children: [
                  if (canRefresh && refreshJobAsync.isLoading)
                    const SizedBox(
                      width: 24,
                      height: 24,
                      child: Padding(
                        padding: EdgeInsets.all(4),
                        child: CircularProgressIndicator(strokeWidth: 2),
                      ),
                    )
                  else if (canRefresh)
                    IconButton(
                      icon: const Icon(Icons.refresh),
                      tooltip: t('jce.comparison.refresh'),
                      onPressed: () => onRefresh(id),
                    ),
                  if (canUseAdvisor)
                    IconButton(
                      icon: const Icon(Icons.chat_bubble_outline),
                      tooltip: t('jce.advisor.title'),
                      onPressed: () {
                        if (advisorKey.currentContext != null) {
                          Scrollable.ensureVisible(advisorKey.currentContext!);
                        }
                      },
                    ),
                ],
              ),
            ],
          ),
          const SizedBox(height: 8),
          if (canViewFreshness) _FreshnessSummaryChip(summaryAsync: freshnessAsync),
          if (canViewFreshness) const SizedBox(height: 8),
          _MatrixTable(
            dimensions: detail.dimensions,
            matrix: matrix,
            freshnessAsync: freshnessAsync,
          ),
          const SizedBox(height: 24),
          if (canViewRecommendations) _RecommendationsPanel(async: recommendationsAsync),
          if (canViewRecommendations) const SizedBox(height: 24),
          if (canViewIngestionJobs) _IngestionJobsPanel(async: jobsAsync),
          if (canViewIngestionJobs) const SizedBox(height: 24),
          if (canUseAdvisor)
            _AdvisorPanel(
              key: advisorKey,
              controller: advisorController,
              async: advisorAsync,
              onAsk: () => onAskAdvisor(id, selectedProfileId),
            ),
          const SizedBox(height: 32),
        ],
      ),
    );
  }
}

class _ItemCard extends StatelessWidget {
  final ResolvedItem item;

  const _ItemCard({required this.item});

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      child: ListTile(
        leading: AvatarWidget(url: item.imageUrl, size: 40),
        title: Text(item.title),
        subtitle: item.description != null ? Text(item.description!) : null,
        trailing: item.url != null ? const Icon(Icons.open_in_new) : null,
        onTap: item.url != null ? () => jcLaunchUrl(context, item.url) : null,
      ),
    );
  }
}

class _ScoreControls extends ConsumerWidget {
  final String comparisonId;
  final String typeSlug;
  final bool canUseProfiles;
  final bool canRecalculateScore;
  final void Function(String, String?, String) onScore;

  const _ScoreControls({
    required this.comparisonId,
    required this.typeSlug,
    required this.canUseProfiles,
    required this.canRecalculateScore,
    required this.onScore,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final selectedProfileId = ref.watch(selectedProfileIdProvider);
    final selectedSmartMode = ref.watch(selectedSmartModeProvider);
    final typeAsync = ref.watch(jceComparisonTypeProvider(typeSlug));
    final profilesAsync = canUseProfiles ? ref.watch(jceProfilesProvider) : null;

    final smartModes = typeAsync.value?.config?.smartModes?.keys.toList() ?? [];
    final modes = ['none', ...smartModes];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (canRecalculateScore && modes.length > 1)
          DropdownButtonHideUnderline(
            child: DropdownButton<String>(
              value: selectedSmartMode,
              isExpanded: true,
              items: modes
                  .map((m) => DropdownMenuItem(value: m, child: Text(_modeLabel(t, m))))
                  .toList(),
              onChanged: (m) {
                if (m != null) {
                  ref.read(selectedSmartModeProvider.notifier).setMode(m);
                }
              },
            ),
          ),
        if (canUseProfiles)
          profilesAsync!.when(
            data: (profiles) {
              final selected = profiles.where((p) => p.id == selectedProfileId).firstOrNull;
              final active = selected ??
                  profiles.where((p) => p.isDefault).firstOrNull ??
                  profiles.firstOrNull;
              if (active == null) {
                return ListTile(
                  contentPadding: EdgeInsets.zero,
                  leading: const Icon(Icons.person_outline),
                  title: Text(t('jce.profile.empty')),
                );
              }
              return ListTile(
                contentPadding: EdgeInsets.zero,
                leading: const Icon(Icons.person_outline),
                title: Text(active.title),
                subtitle: Text(t('jce.profile.selected')),
                onTap: () => _showProfilePicker(context, ref, profiles),
              );
            },
            loading: () => const ListTile(
              contentPadding: EdgeInsets.zero,
              leading: Icon(Icons.person_outline),
              title: Text('—'),
            ),
            error: (_, _) => const SizedBox.shrink(),
          ),
        if (canRecalculateScore) ...[
          const SizedBox(height: 8),
          JcButton(
            label: t('jce.comparison.score'),
            onPressed: () => onScore(comparisonId, selectedProfileId, selectedSmartMode),
          ),
        ],
      ],
    );
  }

  void _showProfilePicker(BuildContext context, WidgetRef ref, List<ComparisonProfile> profiles) {
    final t = ref.read(tProvider);
    showModalBottomSheet(
      context: context,
      builder: (ctx) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Padding(
              padding: const EdgeInsets.all(16),
              child: Text(t('jce.profile.select'), style: JcTypography.heading),
            ),
            ...profiles.map((p) => ListTile(
                  title: Text(p.title),
                  trailing: p.isDefault ? const Icon(Icons.check) : null,
                  onTap: () {
                    ref.read(selectedProfileIdProvider.notifier).select(p.id);
                    Navigator.of(ctx).pop();
                  },
                )),
          ],
        ),
      ),
    );
  }
}

class _ScorePanel extends ConsumerWidget {
  final AsyncValue<ScoreResult?> scoreAsync;

  const _ScorePanel({required this.scoreAsync});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    return scoreAsync.when(
      data: (result) {
        if (result == null) return const SizedBox.shrink();
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              t('jce.score.smart_mode', {'mode': _modeLabel(t, result.smartMode)}),
              style: JcTypography.caption,
            ),
            const SizedBox(height: 8),
            ...result.scores.map((score) => _JapaScoreCard(score: score)),
            if (result.recommendationSummary != null)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: JcPill(
                  label: t(
                    'jce.score.top',
                    {
                      'item_id': result.recommendationSummary!.topItemId,
                      'score': result.recommendationSummary!.topJapaScore.toStringAsFixed(1),
                    },
                  ),
                ),
              ),
          ],
        );
      },
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) => Text(t('jce.score.error', {'error': e.toString()})),
    );
  }
}

class _JapaScoreCard extends StatelessWidget {
  final JapaScore score;

  const _JapaScoreCard({required this.score});

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      child: ExpansionTile(
        title: Text('${score.itemTitle}: ${score.japaScore.toStringAsFixed(1)} / ${score.maxValue}'),
        subtitle: Text(score.eligibleStatus),
        children: [
          Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(score.explanation),
                const SizedBox(height: 8),
                ...score.factorScores.map((fs) => ListTile(
                      dense: true,
                      title: Text(fs.label),
                      trailing: Text('${fs.scoreValue.toStringAsFixed(1)} / ${fs.maxValue}'),
                      subtitle: Text(fs.explanation, maxLines: 2, overflow: TextOverflow.ellipsis),
                    )),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _FreshnessSummaryChip extends ConsumerWidget {
  final AsyncValue<FreshnessSummary?> summaryAsync;

  const _FreshnessSummaryChip({required this.summaryAsync});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    return summaryAsync.when(
      data: (summary) {
        if (summary == null) return const SizedBox.shrink();
        final label =
            '${t('jce.freshness.title')}: ${summary.overallFreshnessScore.toStringAsFixed(0)}% · ${summary.staleCount}/${summary.totalCount} ${t('jce.freshness.stale')}';
        return JcPill(
          label: label,
          color: summary.staleCount > 0 ? JcColors.error.withValues(alpha: 0.1) : null,
        );
      },
      loading: () => const SizedBox.shrink(),
      error: (_, _) => const SizedBox.shrink(),
    );
  }
}

class _MatrixTable extends ConsumerWidget {
  final List<JceDimension> dimensions;
  final List<MatrixItem> matrix;
  final AsyncValue<FreshnessSummary?> freshnessAsync;

  const _MatrixTable({
    required this.dimensions,
    required this.matrix,
    required this.freshnessAsync,
  });

  MatrixDimension? _cellFor(MatrixItem item, JceDimension dim) {
    try {
      return item.dimensions.firstWhere((d) => d.dimensionId == dim.id);
    } catch (_) {
      return null;
    }
  }

  FreshnessCell? _findCell(String itemId, String dimId, FreshnessSummary? summary) {
    final item = summary?.byItem.firstWhere(
      (i) => i.itemId == itemId,
      orElse: () => const FreshnessItem(itemId: '', itemTitle: '', freshnessScore: 0, cells: []),
    );
    if (item == null || item.itemId.isEmpty) return null;
    try {
      return item.cells.firstWhere((c) => c.dimensionId == dimId);
    } catch (_) {
      return null;
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final summary = freshnessAsync.value;

    if (matrix.isEmpty || dimensions.isEmpty) {
      return EmptyState(message: t('jce.comparison.empty_matrix'));
    }

    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      child: DataTable(
        columns: [
          DataColumn(label: Text(t('jce.comparison.matrix_dimension'))),
          ...matrix.map((item) => DataColumn(label: Text(item.title))),
        ],
        rows: dimensions.map((dim) {
          return DataRow(
            cells: [
              DataCell(Text(dim.label)),
              ...matrix.map((item) {
                final cell = _cellFor(item, dim);
                final fresh = _findCell(item.id, dim.id, summary);
                return DataCell(
                  Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(_formatValue(t, cell)),
                      if (fresh != null && fresh.sourceName != null)
                        JcPill(
                          label: '${fresh.freshnessScore.toStringAsFixed(0)}%',
                          color: fresh.isStale ? JcColors.error.withValues(alpha: 0.1) : null,
                        ),
                    ],
                  ),
                );
              }),
            ],
          );
        }).toList(),
      ),
    );
  }

  String _formatValue(String Function(String, [Map<String, String>?]) t, MatrixDimension? cell) {
    if (cell == null) return '—';
    if (cell.value != null) return cell.value!;
    if (cell.valueRating != null) return cell.valueRating!.toStringAsFixed(1);
    if (cell.valueBoolean != null) {
      return cell.valueBoolean! ? t('jce.common.yes') : t('jce.common.no');
    }
    return '—';
  }
}

class _RecommendationsPanel extends ConsumerWidget {
  final AsyncValue<List<ComparisonRecommendation>> async;

  const _RecommendationsPanel({required this.async});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    return async.when(
      data: (items) {
        if (items.isEmpty) return const SizedBox.shrink();
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(t('jce.recommendations.title'), style: JcTypography.heading),
            const SizedBox(height: 8),
            ...items.map((r) => Card(
                  margin: const EdgeInsets.only(bottom: 8),
                  child: ListTile(
                    title: Text(r.text),
                    trailing: r.actionUrl != null ? const Icon(Icons.open_in_new) : null,
                    onTap: r.actionUrl != null ? () => jcLaunchUrl(context, r.actionUrl) : null,
                  ),
                )),
          ],
        );
      },
      loading: () => const SizedBox.shrink(),
      error: (_, _) => const SizedBox.shrink(),
    );
  }
}

class _IngestionJobsPanel extends ConsumerWidget {
  final AsyncValue<List<IngestionJob>> async;

  const _IngestionJobsPanel({required this.async});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    return async.when(
      data: (jobs) {
        if (jobs.isEmpty) return const SizedBox.shrink();
        final running = jobs.where((j) => j.status == 'running' || j.status == 'pending').toList();
        final latest = jobs.firstOrNull;
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(t('jce.jobs.title'), style: JcTypography.heading),
            const SizedBox(height: 8),
            if (latest != null)
              ListTile(
                contentPadding: EdgeInsets.zero,
                title: Text(t('jce.jobs.latest', {'status': latest.status, 'count': latest.refreshedCount.toString()})),
                subtitle: latest.error != null ? Text(latest.error!) : null,
                trailing: running.isNotEmpty
                    ? const SizedBox(width: 20, height: 20, child: CircularProgressIndicator(strokeWidth: 2))
                    : null,
              ),
          ],
        );
      },
      loading: () => const SizedBox.shrink(),
      error: (_, _) => const SizedBox.shrink(),
    );
  }
}

class _AdvisorPanel extends ConsumerWidget {
  final TextEditingController controller;
  final AsyncValue<List<AdvisorStreamChunk>> async;
  final VoidCallback onAsk;

  const _AdvisorPanel({
    super.key,
    required this.controller,
    required this.async,
    required this.onAsk,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final chunks = async.value ?? [];

    final content = chunks
        .where((c) => c.type == 'content' && c.content != null)
        .map((c) => c.content!)
        .join();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(t('jce.advisor.title'), style: JcTypography.heading),
        const SizedBox(height: 8),
        Row(
          children: [
            Expanded(
              child: JcTextField(
                label: t('jce.advisor.ask_hint'),
                controller: controller,
              ),
            ),
            const SizedBox(width: 8),
            IconButton(
              icon: const Icon(Icons.send),
              onPressed: onAsk,
            ),
          ],
        ),
        const SizedBox(height: 8),
        async.when(
          data: (_) => Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: Theme.of(context).colorScheme.surfaceContainerHighest,
              borderRadius: BorderRadius.circular(12),
            ),
            child: content.isEmpty
                ? Text(t('jce.advisor.empty'))
                : SelectableText(content),
          ),
          loading: () => const Center(child: CircularProgressIndicator()),
          error: (e, _) => Text(t('jce.advisor.error', {'error': e.toString()})),
        ),
      ],
    );
  }
}

class _ProfileDrawer extends ConsumerWidget {
  final String comparisonId;
  final VoidCallback onManage;

  const _ProfileDrawer({
    required this.comparisonId,
    required this.onManage,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final profilesAsync = ref.watch(jceProfilesProvider);
    final selected = ref.watch(selectedProfileIdProvider);
    final smartMode = ref.watch(selectedSmartModeProvider);
    final typeSlug = ref.watch(jceComparisonDetailProvider(comparisonId)).value?.typeSlug ?? '';
    final typeAsync = ref.watch(jceComparisonTypeProvider(typeSlug));
    final smartModes = typeAsync.value?.config?.smartModes?.keys.toList() ?? [];

    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(t('jce.profile.title'), style: JcTypography.heading),
            const SizedBox(height: 16),
            if (smartModes.isNotEmpty)
              DropdownButtonHideUnderline(
                child: DropdownButton<String>(
                  value: smartMode,
                  isExpanded: true,
                  items: ['none', ...smartModes]
                      .map((m) => DropdownMenuItem(
                            value: m,
                            child: Text(_modeLabel(t, m)),
                          ))
                      .toList(),
                  onChanged: (m) {
                    if (m != null) {
                      ref.read(selectedSmartModeProvider.notifier).setMode(m);
                    }
                  },
                ),
              ),
            const SizedBox(height: 8),
            profilesAsync.when(
              data: (profiles) => Column(
                mainAxisSize: MainAxisSize.min,
                children: profiles.map((p) => ListTile(
                      title: Text(p.title),
                      leading: Icon(selected == p.id ? Icons.check_circle : Icons.radio_button_unchecked),
                      onTap: () => ref.read(selectedProfileIdProvider.notifier).select(p.id),
                    )).toList(),
              ),
              loading: () => const LoadingShimmer(),
              error: (e, _) => JcErrorWidget(error: e, onRetry: () => ref.invalidate(jceProfilesProvider)),
            ),
            const SizedBox(height: 16),
            JcButton(label: t('jce.profile.manage'), onPressed: onManage),
          ],
        ),
      ),
    );
  }
}

String _modeLabel(String Function(String, [Map<String, String>?]) t, String mode) {
  if (mode == 'none') return t('jce.smart_mode.none');
  final localized = t('jce.smart_mode.$mode');
  if (localized != mode) return localized;
  // Fallback: turn "cost_optimized" into "Cost Optimized".
  return mode
      .replaceAll('_', ' ')
      .split(' ')
      .map((word) => word.isEmpty ? word : '${word[0].toUpperCase()}${word.substring(1)}')
      .join(' ');
}
