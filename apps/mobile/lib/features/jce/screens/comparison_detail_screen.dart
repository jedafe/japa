import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
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
  Timer? _pollTimer;
  int _pollCount = 0;

  @override
  void dispose() {
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

  void _refresh(String comparisonId) {
    ref.read(refreshComparisonProvider(comparisonId).notifier).refresh();
    _startPolling();
  }

  @override
  Widget build(BuildContext context) {
    final t = ref.watch(tProvider);
    final detailAsync = ref.watch(jceComparisonDetailProvider(widget.id));

    return Scaffold(
      appBar: AppBar(
        title: Text(detailAsync.value?.content.title ?? t('jce.comparison.title')),
      ),
      body: detailAsync.when(
        data: (detail) {
          if (detail == null) {
            return EmptyState(message: t('jce.comparison.not_found'));
          }
          return _JceComparisonDetailBody(
            id: widget.id,
            detail: detail,
            onRefresh: _refresh,
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
}

class _JceComparisonDetailBody extends ConsumerWidget {
  final String id;
  final JceComparisonDetail detail;
  final void Function(String) onRefresh;

  const _JceComparisonDetailBody({
    required this.id,
    required this.detail,
    required this.onRefresh,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final matrixAsync = ref.watch(jceMatrixProvider(id));
    final capsAsync = ref.watch(jceCapabilitiesProvider);
    final caps = capsAsync.value;

    final canViewFreshness = caps?.canViewFreshness ?? false;
    final canRefresh = caps?.canRefresh ?? false;
    final canViewIngestionJobs = caps?.canViewIngestionJobs ?? false;

    final refreshJobAsync = canRefresh
        ? ref.watch(refreshComparisonProvider(id))
        : const AsyncData<IngestionJob?>(null);
    final freshnessAsync = canViewFreshness
        ? ref.watch(jceFreshnessProvider(id))
        : const AsyncData<FreshnessSummary?>(null);
    final jobsAsync = canViewIngestionJobs
        ? ref.watch(jceIngestionJobsProvider(id))
        : const AsyncData<List<IngestionJob>>([]);

    final resolvedItems = detail.resolvedItems;
    final matrix = matrixAsync.value ?? [];

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
          if (canViewIngestionJobs) _IngestionJobsPanel(async: jobsAsync),
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
