import 'package:japaconnect_core/japaconnect_core.dart';

class FakeJcApiClient extends JcApiClient {
  FakeJcApiClient()
      : super(
          baseUrl: 'http://localhost:8080/api/v1',
          tokenStore: TokenStore(),
          deviceId: DeviceId(),
        );

  @override
  Future<T> get<T>(
    String path, {
    Map<String, dynamic>? query,
    required T Function(dynamic) parse,
  }) async {
    return parse(_dataFor(path));
  }

  @override
  Future<T> post<T>(
    String path, {
    Map<String, dynamic>? body,
    required T Function(dynamic) parse,
  }) async {
    return parse(_dataFor(path, body: body));
  }

  @override
  Future<T> patch<T>(
    String path, {
    Map<String, dynamic>? body,
    required T Function(dynamic) parse,
  }) async {
    return parse(_dataFor(path, body: body));
  }

  @override
  Future<void> delete(String path) async {}

  @override
  Future<PaginatedList<T>> getList<T>(
    String path, {
    Map<String, dynamic>? query,
    required T Function(dynamic) parse,
  }) async {
    final data = _listDataFor(path, query: query);
    final items = (data['data'] as List).map(parse).cast<T>().toList();
    final meta = data['meta'] == null
        ? null
        : ListMeta.fromJson(data['meta'] as Map<String, dynamic>);
    return PaginatedList<T>(items: items, meta: meta);
  }

  @override
  Stream<Map<String, dynamic>> streamSse(
    String path, {
    String method = 'POST',
    Map<String, dynamic>? body,
  }) async* {}

  dynamic _dataFor(String path, {Map<String, dynamic>? body}) {
    if (path == '/feature-flags') {
      return {
        'feature.jce': true,
        'feature.jce.refresh': true,
      };
    }
    if (path == '/me/permissions') {
      return {
        'permissions': [
          'jce.refresh.trigger',
          'jce.ingestion_job.view',
        ],
      };
    }
    if (RegExp(r'^/jce/comparisons/[^/]+$').hasMatch(path)) {
      return _sampleDetail();
    }
    if (RegExp(r'^/jce/comparisons/[^/]+/matrix$').hasMatch(path)) {
      return [];
    }
    if (RegExp(r'^/jce/comparisons/[^/]+/freshness$').hasMatch(path)) {
      return _sampleFreshness();
    }
    if (RegExp(r'^/jce/comparisons/[^/]+/ingestion-jobs$').hasMatch(path)) {
      return [];
    }
    return null;
  }

  Map<String, dynamic> _listDataFor(String path, {Map<String, dynamic>? query}) {
    if (path == '/jce/comparisons') {
      final comparisons = _sampleComparisons();
      return {
        'data': comparisons,
        'meta': {
          'total': comparisons.length,
          'page': 1,
          'limit': 12,
          'page_size_options': [12, 24, 48, 96],
          'sort_options': ['created_at', 'title'],
        },
      };
    }
    if (RegExp(r'^/jce/comparisons/[^/]+/ingestion-jobs$').hasMatch(path)) {
      return {'data': [], 'meta': {}};
    }
    return {'data': [], 'meta': {}};
  }

  List<dynamic> _sampleComparisons() => [
        {
          'id': 'c1',
          'title': 'Lagos vs London',
          'slug': 'lagos-vs-london',
          'status': 'published',
          'visibility': 'public',
          'depth': 'detailed',
        },
        {
          'id': 'c2',
          'title': 'Toronto vs Berlin',
          'slug': 'toronto-vs-berlin',
          'status': 'published',
          'visibility': 'public',
          'depth': 'quick',
        },
      ];

  Map<String, dynamic> _sampleDetail() => {
        'content': {
          'id': 'c1',
          'title': 'Lagos vs London',
          'slug': 'lagos-vs-london',
          'body_html': '<p>Body HTML</p>',
          'status': 'published',
          'visibility': 'public',
        },
        'metadata': {
          'content_id': 'c1',
          'depth': 'detailed',
        },
        'items': [
          {
            'id': 'i1',
            'title': 'Lagos',
            'item_ref': {
              'type': 'space',
              'id': 's1',
              'url': 'https://example.com/lagos',
            },
          },
          {
            'id': 'i2',
            'title': 'London',
            'item_ref': {
              'type': 'space',
              'id': 's2',
            },
          },
        ],
        'dimensions': [
          {'id': 'd1', 'label': 'Cost'},
        ],
        'type_slug': 'city-vs-city',
      };

  Map<String, dynamic> _sampleFreshness() => {
        'content_id': 'c1',
        'overall_freshness_score': 95,
        'stale_count': 0,
        'total_count': 2,
        'by_item': [],
      };
}
