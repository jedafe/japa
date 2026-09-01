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
// Detail / matrix / profiles
// ---------------------------------------------------------------------------

final jceComparisonTypeProvider =
    FutureProvider.autoDispose.family<ComparisonType?, String>((ref, typeSlug) async {
  final service = ref.watch(jceServiceProvider);
  return service.getComparisonType(typeSlug);
});

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

final jceProfilesProvider =
    FutureProvider.autoDispose<List<ComparisonProfile>>((ref) async {
  final service = ref.watch(jceServiceProvider);
  return service.listProfiles();
});

class SelectedProfileIdNotifier extends Notifier<String?> {
  @override
  String? build() => null;

  void select(String? id) => state = id;
}

final selectedProfileIdProvider =
    NotifierProvider<SelectedProfileIdNotifier, String?>(SelectedProfileIdNotifier.new);

class SelectedSmartModeNotifier extends Notifier<String> {
  @override
  String build() => 'none';

  void setMode(String mode) => state = mode;
}

final selectedSmartModeProvider =
    NotifierProvider<SelectedSmartModeNotifier, String>(SelectedSmartModeNotifier.new);

// ---------------------------------------------------------------------------
// Score & recommendations
// ---------------------------------------------------------------------------

final jceRecommendationsProvider =
    FutureProvider.autoDispose.family<List<ComparisonRecommendation>, String>((ref, id) async {
  final service = ref.watch(jceServiceProvider);
  final profileId = ref.watch(selectedProfileIdProvider);
  return service.listRecommendations(id, profileID: profileId);
});

class ScoreComparisonNotifier extends AsyncNotifier<ScoreResult?> {
  final String comparisonId;

  ScoreComparisonNotifier(this.comparisonId);

  JceService get _service => ref.read(jceServiceProvider);

  @override
  Future<ScoreResult?> build() async {
    final profileId = ref.watch(selectedProfileIdProvider);
    return _service.getScore(comparisonId, profileID: profileId);
  }

  Future<void> score({
    String? profileId,
    String? smartMode,
    bool? force,
  }) async {
    state = const AsyncLoading();
    try {
      final result = await _service.scoreComparison(
        comparisonId,
        profileID: profileId,
        smartMode: smartMode,
        force: force ?? false,
      );
      state = AsyncData(result);
      ref.invalidate(jceRecommendationsProvider(comparisonId));
    } catch (e, st) {
      state = AsyncError(e, st);
    }
  }
}

final scoreComparisonProvider =
    AsyncNotifierProvider.autoDispose.family<ScoreComparisonNotifier, ScoreResult?, String>(
  (arg) => ScoreComparisonNotifier(arg),
);

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
// Advisor stream
// ---------------------------------------------------------------------------

class JceAdvisorArgs {
  final String comparisonId;
  final String question;
  final String? profileId;

  const JceAdvisorArgs({
    required this.comparisonId,
    required this.question,
    this.profileId,
  });

  @override
  bool operator ==(Object other) =>
      other is JceAdvisorArgs &&
      other.comparisonId == comparisonId &&
      other.question == question &&
      other.profileId == profileId;

  @override
  int get hashCode => Object.hash(comparisonId, question, profileId);
}

class JceAdvisorArgsNotifier extends Notifier<JceAdvisorArgs?> {
  final String comparisonId;

  JceAdvisorArgsNotifier(this.comparisonId);

  @override
  JceAdvisorArgs? build() => null;

  void setArgs(JceAdvisorArgs? args) => state = args;
}

final jceAdvisorArgsProvider =
    NotifierProvider.autoDispose.family<JceAdvisorArgsNotifier, JceAdvisorArgs?, String>(
  (arg) => JceAdvisorArgsNotifier(arg),
);

final jceAdvisorStreamProvider =
    StreamProvider.autoDispose.family<List<AdvisorStreamChunk>, String>((ref, id) async* {
  final args = ref.watch(jceAdvisorArgsProvider(id));
  if (args == null) {
    yield [];
    return;
  }

  final service = ref.watch(jceServiceProvider);
  final stream = service.streamAdvisor(
    id,
    args.question,
    profileID: args.profileId,
  );

  final chunks = <AdvisorStreamChunk>[];
  await for (final chunk in stream) {
    chunks.add(chunk);
    yield List.unmodifiable(chunks);
  }
});

// ---------------------------------------------------------------------------
// Capabilities (feature flags + permissions)
// ---------------------------------------------------------------------------

class JceCapabilities {
  final FeatureFlags flags;
  final List<String> permissions;

  const JceCapabilities({required this.flags, required this.permissions});

  bool _hasPerm(String key) => permissions.contains(key);

  bool get canViewScore =>
      flags.isEnabled('feature.jce.scoring') &&
      (_hasPerm('jce.score.view') || _hasPerm('jce.score.recalculate'));

  bool get canRecalculateScore =>
      flags.isEnabled('feature.jce.scoring') && _hasPerm('jce.score.recalculate');

  bool get canViewRecommendations =>
      flags.isEnabled('feature.jce.scoring') &&
      _hasPerm('jce.recommendation.view');

  bool get canViewFreshness =>
      flags.isEnabled('feature.jce') &&
      (_hasPerm('jce.score.view') || _hasPerm('jce.score.recalculate'));

  bool get canRefresh =>
      flags.isEnabled('feature.jce.refresh') && _hasPerm('jce.refresh.trigger');

  bool get canViewIngestionJobs =>
      flags.isEnabled('feature.jce.refresh') && _hasPerm('jce.ingestion_job.view');

  bool get canUseAdvisor =>
      flags.isEnabled('feature.jce.ai_advisor') && _hasPerm('jce.ai_advisor.use');

  bool get canUseProfiles =>
      flags.isEnabled('feature.jce.profiles') && _hasPerm('jce.profile.list');
}

final jceCapabilitiesProvider = FutureProvider.autoDispose<JceCapabilities>((ref) async {
  final service = ref.watch(jceServiceProvider);
  final flags = await service.getFeatureFlags();
  final permissions = await service.getUserPermissions();
  return JceCapabilities(flags: flags, permissions: permissions);
});
