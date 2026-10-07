import 'package:japaconnect_core/japaconnect_core.dart';

class SearchEntitiesOptions {
  final int? page;
  final int? pageSize;
  final int? offset;

  const SearchEntitiesOptions({this.page, this.pageSize, this.offset});
}

class JceService {
  final JcApiClient _client;

  JceService(this._client);

  Future<FeatureFlags> getFeatureFlags() async {
    return _client.get<FeatureFlags>(
      '/feature-flags',
      parse: (json) => FeatureFlags.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<List<String>> getUserPermissions() async {
    return _client.get<List<String>>(
      '/me/permissions',
      parse: (json) {
        final permissions =
            (json as Map<String, dynamic>)['permissions'] as List<dynamic>? ?? [];
        return permissions.whereType<String>().toList();
      },
    );
  }

  Future<List<ComparisonType>> listComparisonTypes() async {
    return _client.get<List<ComparisonType>>(
      '/jce/comparison-types',
      parse: (json) => (json as List)
          .map((e) => ComparisonType.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }

  Future<ComparisonType?> getComparisonType(String slug) async {
    return _client.get<ComparisonType?>(
      '/jce/comparison-types/$slug',
      parse: (json) => json == null ? null : ComparisonType.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<List<ResolvedItem>> searchEntities(
    String type,
    String query, {
    Map<String, String> filters = const {},
    SearchEntitiesOptions options = const SearchEntitiesOptions(),
  }) async {
    final queryParams = <String, dynamic>{'q': query};
    if (options.pageSize != null) queryParams['page_size'] = options.pageSize.toString();
    if (options.page != null) {
      queryParams['page'] = options.page.toString();
    } else if (options.offset != null) {
      queryParams['offset'] = options.offset.toString();
    }
    for (final entry in filters.entries) {
      if (entry.value.isNotEmpty) queryParams[entry.key] = entry.value;
    }
    return _client.get<List<ResolvedItem>>(
      '/jce/entities/$type',
      query: queryParams,
      parse: (json) => (json as List)
          .map((e) => ResolvedItem.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }

  Future<List<({String id, String name, String slug})>> searchSpaces(
    String query, {
    String? level,
    int limit = 10,
  }) async {
    final result = await _client.get<List<dynamic>>(
      '/spaces',
      query: {'q': query, 'page_size': limit.toString(), 'level': ?level},
      parse: (json) => json as List,
    );
    return result
        .whereType<Map<String, dynamic>>()
        .map((e) {
          return (
            id: e['id'] as String? ?? '',
            name: e['name'] as String? ?? '',
            slug: e['slug'] as String? ?? '',
          );
        })
        .where((s) => s.id.isNotEmpty && s.name.isNotEmpty)
        .toList();
  }

  Future<ResolvedItem?> resolveEntity(String type, String id) async {
    return _client.get<ResolvedItem?>(
      '/jce/entities/$type/$id',
      parse: (json) => json == null ? null : ResolvedItem.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<String?> createGuestSelection({
    required String typeSlug,
    required List<Map<String, dynamic>> itemRefs,
  }) async {
    final token = await _client.post<SelectionToken?>(
      '/jce/selections',
      body: {'type_slug': typeSlug, 'item_refs': itemRefs},
      parse: (json) => json == null ? null : SelectionToken.fromJson(json as Map<String, dynamic>),
    );
    return token?.token;
  }

  Future<JceComparisonSummary?> createQuickComparisonFromToken(
    String token,
    String typeID, {
    String? title,
  }) async {
    return _client.post<JceComparisonSummary?>(
      '/jce/comparisons/from-token',
      body: {
        'token': token,
        'type_id': typeID,
        'title': title ?? 'Quick comparison',
      },
      parse: (json) =>
          json == null ? null : JceComparisonSummary.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<JceComparisonSummary?> createQuickComparison(CreateQuickComparisonInput input) async {
    return _client.post<JceComparisonSummary?>(
      '/jce/comparisons',
      body: {
        ...input.toJson(),
        'depth': 'quick',
        'status': 'draft',
        'visibility': 'private',
      },
      parse: (json) =>
          json == null ? null : JceComparisonSummary.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<JceComparisonSummary?> getQuickComparison(String slug) async {
    return _client.get<JceComparisonSummary?>(
      '/jce/comparisons/$slug',
      parse: (json) =>
          json == null ? null : JceComparisonSummary.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<PaginatedList<JceComparisonSummary>> listComparisons({
    Map<String, dynamic>? query,
  }) async {
    return _client.getList<JceComparisonSummary>(
      '/jce/comparisons',
      query: query,
      parse: (json) => JceComparisonSummary.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<JceComparisonDetail?> getComparison(String slug) async {
    return _client.get<JceComparisonDetail?>(
      '/jce/comparisons/$slug',
      parse: (json) =>
          json == null ? null : JceComparisonDetail.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<List<MatrixItem>> getMatrix(String slug) async {
    return _client.get<List<MatrixItem>>(
      '/jce/comparisons/$slug/matrix',
      parse: (json) => (json as List)
          .map((e) => MatrixItem.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }

  Future<void> deleteComparison(String id) async {
    await _client.delete('/jce/comparisons/$id');
  }

  Future<JceComparisonSummary?> promoteComparison(String id) async {
    return _client.post<JceComparisonSummary?>(
      '/jce/comparisons/$id/promote',
      parse: (json) =>
          json == null ? null : JceComparisonSummary.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<List<DataSource>> listDataSources() async {
    return _client.get<List<DataSource>>(
      '/jce/data-sources',
      parse: (json) => (json as List)
          .map((e) => DataSource.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }

  Future<FreshnessSummary?> getFreshness(String comparisonID) async {
    return _client.get<FreshnessSummary?>(
      '/jce/comparisons/$comparisonID/freshness',
      parse: (json) =>
          json == null ? null : FreshnessSummary.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<IngestionJob?> refreshComparison(
    String comparisonID, {
    String? sourceKind,
  }) async {
    final body = <String, dynamic>{};
    if (sourceKind != null && sourceKind.isNotEmpty) body['source_kind'] = sourceKind;
    return _client.post<IngestionJob?>(
      '/jce/comparisons/$comparisonID/refresh',
      body: body,
      parse: (json) => json == null ? null : IngestionJob.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<List<IngestionJob>> listIngestionJobs(String comparisonID) async {
    return _client.get<List<IngestionJob>>(
      '/jce/comparisons/$comparisonID/ingestion-jobs',
      parse: (json) => (json as List)
          .map((e) => IngestionJob.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }

}
