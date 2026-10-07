import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/jce/services/jce_service.dart';

final jceServiceProvider = Provider<JceService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return JceService(client);
});

// ---------------------------------------------------------------------------
// List pagination
// ---------------------------------------------------------------------------

class JceComparisonListState {
  final List<JceComparisonSummary> items;
  final int page;
  final int pageSize;
  final bool hasMore;
  final bool isLoadingMore;
  final ListMeta? meta;
  final Map<String, dynamic>? query;

  const JceComparisonListState({
    required this.items,
    required this.page,
    required this.pageSize,
    required this.hasMore,
    this.isLoadingMore = false,
    this.meta,
    this.query,
  });

  JceComparisonListState copyWith({
    List<JceComparisonSummary>? items,
    int? page,
    int? pageSize,
    bool? hasMore,
    bool? isLoadingMore,
    ListMeta? meta,
    Map<String, dynamic>? query,
  }) {
    return JceComparisonListState(
      items: items ?? this.items,
      page: page ?? this.page,
      pageSize: pageSize ?? this.pageSize,
      hasMore: hasMore ?? this.hasMore,
      isLoadingMore: isLoadingMore ?? this.isLoadingMore,
      meta: meta ?? this.meta,
      query: query ?? this.query,
    );
  }
}

class JceComparisonListNotifier extends AsyncNotifier<JceComparisonListState> {
  static const int defaultPageSize = 12;

  @override
  Future<JceComparisonListState> build() async {
    return _fetch(page: 1, pageSize: defaultPageSize);
  }

  Future<void> refresh({Map<String, dynamic>? query}) async {
    final current = state.value;
    state = const AsyncLoading();
    try {
      state = AsyncData(await _fetch(
        page: 1,
        pageSize: current?.pageSize ?? defaultPageSize,
        query: query,
      ));
    } catch (e, st) {
      state = AsyncError(e, st);
    }
  }

  Future<void> loadMore() async {
    final current = state.value;
    if (current == null || current.isLoadingMore || !current.hasMore) return;

    state = AsyncData(current.copyWith(isLoadingMore: true));
    try {
      final next = await _fetch(
        page: current.page + 1,
        pageSize: current.pageSize,
        query: current.query,
      );
      final allItems = [...current.items, ...next.items];
      final totalLoaded = (current.page * current.pageSize) + next.items.length;
      final hasMore = _hasMore(next, totalLoaded);
      state = AsyncData(next.copyWith(
        items: allItems,
        page: next.page,
        isLoadingMore: false,
        hasMore: hasMore,
        query: current.query,
      ));
    } catch (e, st) {
      // Keep the already-loaded items and stop trying to load more.
      final current = state.value;
      if (current != null) {
        state = AsyncData(current.copyWith(isLoadingMore: false, hasMore: false));
      } else {
        state = AsyncError(e, st);
      }
    }
  }

  Future<JceComparisonListState> _fetch({
    required int page,
    required int pageSize,
    Map<String, dynamic>? query,
  }) async {
    final service = ref.read(jceServiceProvider);
    final q = <String, dynamic>{
      'page': page.toString(),
      'page_size': pageSize.toString(),
    };
    if (query != null) q.addAll(query);
    final result = await service.listComparisons(query: q);
    final baseState = JceComparisonListState(
      items: result.items,
      page: page,
      pageSize: pageSize,
      hasMore: false,
      meta: result.meta,
      query: query,
    );
    final totalLoaded = ((page - 1) * pageSize) + result.items.length;
    final hasMore = _hasMore(baseState, totalLoaded);
    return baseState.copyWith(hasMore: hasMore);
  }

  bool _hasMore(JceComparisonListState result, int totalLoaded) {
    if (result.meta != null && result.meta!.total != null) {
      return totalLoaded < result.meta!.total!;
    }
    // Without a total count, a full page means there may be another.
    return result.items.length >= result.pageSize;
  }

  Future<void> setPageSize(int pageSize) async {
    final current = state.value;
    state = const AsyncLoading();
    try {
      state = AsyncData(await _fetch(
        page: 1,
        pageSize: pageSize,
        query: current?.query,
      ));
    } catch (e, st) {
      state = AsyncError(e, st);
    }
  }

  Future<void> setQuery(Map<String, dynamic>? query) async {
    final current = state.value;
    state = const AsyncLoading();
    try {
      state = AsyncData(await _fetch(
        page: 1,
        pageSize: current?.pageSize ?? defaultPageSize,
        query: query,
      ));
    } catch (e, st) {
      state = AsyncError(e, st);
    }
  }
}

final jceComparisonListProvider =
    AsyncNotifierProvider.autoDispose<JceComparisonListNotifier, JceComparisonListState>(
  JceComparisonListNotifier.new,
);

// ---------------------------------------------------------------------------
// Detail / matrix
// ---------------------------------------------------------------------------

final jceComparisonDetailProvider =
    FutureProvider.autoDispose.family<JceComparisonDetail?, String>((ref, id) async {
  final service = ref.watch(jceServiceProvider);
  return service.getComparison(id);
});

final jceMatrixProvider =
    FutureProvider.autoDispose.family<List<MatrixItem>, String>((ref, id) async {
  final service = ref.watch(jceServiceProvider);
  return service.getMatrix(id);
});

// ---------------------------------------------------------------------------
// Freshness & refresh
// ---------------------------------------------------------------------------

final jceFreshnessProvider =
    FutureProvider.autoDispose.family<FreshnessSummary?, String>((ref, id) async {
  final service = ref.watch(jceServiceProvider);
  return service.getFreshness(id);
});

final jceIngestionJobsProvider =
    FutureProvider.autoDispose.family<List<IngestionJob>, String>((ref, id) async {
  final service = ref.watch(jceServiceProvider);
  return service.listIngestionJobs(id);
});

class RefreshComparisonNotifier extends AsyncNotifier<IngestionJob?> {
  final String comparisonId;

  RefreshComparisonNotifier(this.comparisonId);

  @override
  Future<IngestionJob?> build() async => null;

  Future<void> refresh({String? sourceKind}) async {
    state = const AsyncLoading();
    try {
      final service = ref.read(jceServiceProvider);
      final job = await service.refreshComparison(comparisonId, sourceKind: sourceKind);
      state = AsyncData(job);
      ref.invalidate(jceIngestionJobsProvider(comparisonId));
      ref.invalidate(jceFreshnessProvider(comparisonId));
    } catch (e, st) {
      state = AsyncError(e, st);
    }
  }
}

final refreshComparisonProvider =
    AsyncNotifierProvider.autoDispose.family<RefreshComparisonNotifier, IngestionJob?, String>(
  (arg) => RefreshComparisonNotifier(arg),
);

// ---------------------------------------------------------------------------
// Data sources
// ---------------------------------------------------------------------------

final jceDataSourcesProvider =
    FutureProvider.autoDispose<List<DataSource>>((ref) async {
  final service = ref.watch(jceServiceProvider);
  return service.listDataSources();
});

// ---------------------------------------------------------------------------
// Capabilities (feature flags + permissions)
// ---------------------------------------------------------------------------

class JceCapabilities {
  final FeatureFlags flags;
  final List<String> permissions;

  const JceCapabilities({required this.flags, required this.permissions});

  bool _hasPerm(String key) => permissions.contains(key);

  bool get canViewFreshness => flags.isEnabled('feature.jce.refresh');

  bool get canRefresh =>
      flags.isEnabled('feature.jce.refresh') && _hasPerm('jce.refresh.trigger');

  bool get canViewIngestionJobs =>
      flags.isEnabled('feature.jce.refresh') && _hasPerm('jce.ingestion_job.view');
}

final jceCapabilitiesProvider = FutureProvider.autoDispose<JceCapabilities>((ref) async {
  final service = ref.watch(jceServiceProvider);
  final flags = await service.getFeatureFlags();
  final permissions = await service.getUserPermissions();
  return JceCapabilities(flags: flags, permissions: permissions);
});
