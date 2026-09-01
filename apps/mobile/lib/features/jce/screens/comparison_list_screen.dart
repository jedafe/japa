import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/jce/providers/jce_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class JceComparisonListScreen extends ConsumerStatefulWidget {
  const JceComparisonListScreen({super.key});

  @override
  ConsumerState<JceComparisonListScreen> createState() => _JceComparisonListScreenState();
}

class _JceComparisonListScreenState extends ConsumerState<JceComparisonListScreen> {
  final _scrollController = ScrollController();

  @override
  void initState() {
    super.initState();
    _scrollController.addListener(_onScroll);
  }

  @override
  void dispose() {
    _scrollController.dispose();
    super.dispose();
  }

  void _onScroll() {
    final pos = _scrollController.position;
    if (pos.pixels >= pos.maxScrollExtent - 200) {
      ref.read(jceComparisonListProvider.notifier).loadMore();
    }
  }

  Future<void> _refresh() async {
    final current = ref.read(jceComparisonListProvider).value;
    await ref.read(jceComparisonListProvider.notifier).refresh(query: current?.query);
  }

  Future<void> _changePageSize(int pageSize) async {
    await ref.read(jceComparisonListProvider.notifier).setPageSize(pageSize);
  }

  Future<void> _changeSort(String sort) async {
    final current = ref.read(jceComparisonListProvider).value;
    final query = {...?current?.query, 'sort': sort};
    await ref.read(jceComparisonListProvider.notifier).setQuery(query);
  }

  @override
  Widget build(BuildContext context) {
    final t = ref.watch(tProvider);
    final state = ref.watch(jceComparisonListProvider);

    return Scaffold(
      appBar: AppBar(
        title: Text(t('jce.comparisons.title')),
        actions: [
          state.when(
            data: (data) => _ListControls(
              state: data,
              onPageSize: _changePageSize,
              onSort: _changeSort,
            ),
            loading: () => const SizedBox.shrink(),
            error: (_, _) => const SizedBox.shrink(),
          ),
        ],
      ),
      body: state.when(
        data: (data) => RefreshIndicator(
          onRefresh: _refresh,
          child: ListView.builder(
            controller: _scrollController,
            physics: const AlwaysScrollableScrollPhysics(),
            itemCount: data.items.length + (data.isLoadingMore ? 1 : 0),
            itemBuilder: (_, i) {
              if (i >= data.items.length) {
                return const Padding(
                  padding: EdgeInsets.all(16),
                  child: Center(child: CircularProgressIndicator()),
                );
              }
              final item = data.items[i];
              return EntityCard(
                item: item.toCardMap(),
                onTap: () => context.push('/comparisons/${item.id}'),
              );
            },
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(jceComparisonListProvider),
        ),
      ),
    );
  }
}

class _ListControls extends ConsumerWidget {
  final JceComparisonListState state;
  final ValueChanged<int> onPageSize;
  final ValueChanged<String> onSort;

  const _ListControls({
    required this.state,
    required this.onPageSize,
    required this.onSort,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final pageSizeOptions = state.meta?.pageSizeOptions?.isNotEmpty == true
        ? state.meta!.pageSizeOptions!
        : [JceComparisonListNotifier.defaultPageSize];
    final sortOptions = state.meta?.sortOptions?.isNotEmpty == true
        ? state.meta!.sortOptions!
        : <String>[];

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (sortOptions.isNotEmpty)
          PopupMenuButton<String>(
            icon: const Icon(Icons.sort),
            tooltip: t('jce.comparisons.sort'),
            onSelected: onSort,
            itemBuilder: (_) => sortOptions
                .map((s) => PopupMenuItem(
                      value: s,
                      child: Text(_sortLabel(s)),
                    ))
                .toList(),
          ),
        PopupMenuButton<int>(
          icon: const Icon(Icons.list),
          tooltip: t('jce.comparisons.page_size'),
          onSelected: onPageSize,
          itemBuilder: (_) => pageSizeOptions
              .map((s) => PopupMenuItem(
                    value: s,
                    child: Text(t('jce.comparisons.page_size_value', {'size': s.toString()})),
                  ))
              .toList(),
        ),
      ],
    );
  }

  String _sortLabel(String sort) {
    // Title-case and replace underscores for display.
    return sort
        .replaceAll('_', ' ')
        .split(' ')
        .map((w) => w.isEmpty ? w : '${w[0].toUpperCase()}${w.substring(1)}')
        .join(' ');
  }
}
