import 'package:japaconnect_core/japaconnect_core.dart';

class FakeJcApiClient extends JcApiClient {
  final bool emptyProfiles;

  FakeJcApiClient({this.emptyProfiles = false})
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
  }) async* {
    yield {'type': 'content', 'content': 'Hello from advisor'};
  }

  dynamic _dataFor(String path, {Map<String, dynamic>? body}) {
    if (path == '/feature-flags') {
      return {
        'feature.jce': true,
        'feature.jce.scoring': true,
        'feature.jce.refresh': true,
        'feature.jce.ai_advisor': true,
        'feature.jce.profiles': true,
      };
    }
    if (path == '/me/permissions') {
      return {
        'permissions': [
          'jce.score.view',
          'jce.score.recalculate',
          'jce.recommendation.view',
          'jce.refresh.trigger',
          'jce.ingestion_job.view',
          'jce.ai_advisor.use',
          'jce.profile.list',
          'jce.profile.create',
          'jce.profile.update',
          'jce.profile.delete',
        ],
      };
    }
    if (RegExp(r'^/jce/comparisons/[^/]+$').hasMatch(path)) {
      return _sampleDetail();
    }
    if (RegExp(r'^/jce/comparisons/[^/]+/matrix$').hasMatch(path)) {
      return [];
    }
    if (RegExp(r'^/jce/comparisons/[^/]+/score$').hasMatch(path)) {
      return _sampleScore();
    }
    if (RegExp(r'^/jce/comparisons/[^/]+/recommendations$').hasMatch(path)) {
      return _sampleRecommendations();
    }
    if (RegExp(r'^/jce/comparisons/[^/]+/freshness$').hasMatch(path)) {
      return _sampleFreshness();
    }
    if (RegExp(r'^/jce/comparisons/[^/]+/ingestion-jobs$').hasMatch(path)) {
      return [];
    }
    if (RegExp(r'^/jce/comparison-types/[^/]+$').hasMatch(path)) {
      return _sampleComparisonType();
    }
    if (path == '/jce/profiles') {
      if (body != null) {
        // create profile
        return _sampleProfiles().first;
      }
      return emptyProfiles ? [] : _sampleProfiles();
    }
    if (RegExp(r'^/jce/profiles/[^/]+$').hasMatch(path)) {
      return _sampleProfiles().first;
    }
    if (RegExp(r'^/jce/profiles/[^/]+/default$').hasMatch(path)) {
      return null;
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
    if (path == '/jce/profiles') {
      final profiles = emptyProfiles ? [] : _sampleProfiles();
      return {
        'data': profiles,
        'meta': {},
      };
    }
    if (RegExp(r'^/jce/comparisons/[^/]+/recommendations$').hasMatch(path)) {
      return {'data': _sampleRecommendations(), 'meta': {}};
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

  Map<String, dynamic> _sampleComparisonType() => {
        'id': 't1',
        'name': 'City vs City',
        'slug': 'city-vs-city',
        'item_label': 'City',
        'config': {
          'smart_modes': {'cost_optimized': {}, 'family_friendly': {}},
        },
      };

  List<dynamic> _sampleProfiles() => [
        {
          'id': 'p1',
          'title': 'Solo mover',
          'target_space_ids': ['s1'],
          'goals': [],
          'preferences': {},
          'is_default': true,
          'is_template': false,
        },
      ];

  Map<String, dynamic> _sampleScore() => {
        'content_id': 'c1',
        'smart_mode': 'none',
        'scores': [
          {
            'item_id': 'i1',
            'item_title': 'Lagos',
            'japa_score': 85,
            'max_value': 100,
            'eligible_status': 'eligible',
            'factor_scores': [],
            'explanation': 'Great match',
          },
        ],
        'recommendations': [],
        'weights_snapshot': {},
        'version': 1,
      };

  List<dynamic> _sampleRecommendations() => [
        {
          'id': 'r1',
          'text': 'Consider Lagos for lower cost of living',
          'priority': 1,
        },
      ];

  Map<String, dynamic> _sampleFreshness() => {
        'content_id': 'c1',
        'overall_freshness_score': 95,
        'stale_count': 0,
        'total_count': 2,
        'by_item': [],
      };
}
