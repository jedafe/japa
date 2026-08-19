import 'package:japaconnect_core/japaconnect_core.dart';

class ContentService {
  final JcApiClient _client;
  ContentService(this._client);

  Future<List<Map<String, dynamic>>> listBlog() async {
    return _client.get<List<Map<String, dynamic>>>(
      '/blog',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
  }

  Future<Map<String, dynamic>> getBlog(String slug) async {
    return _client.get<Map<String, dynamic>>(
      '/blog/$slug',
      parse: (json) => json as Map<String, dynamic>,
    );
  }

  Future<List<Map<String, dynamic>>> listGuides() async {
    return _client.get<List<Map<String, dynamic>>>(
      '/content',
      query: {'type': 'guide'},
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
  }

  Future<Map<String, dynamic>> getGuide(String slug) async {
    return _client.get<Map<String, dynamic>>(
      '/content/$slug',
      parse: (json) => json as Map<String, dynamic>,
    );
  }

  Future<List<Map<String, dynamic>>> listComparisons() async {
    return _client.get<List<Map<String, dynamic>>>(
      '/comparisons',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
  }

  Future<Map<String, dynamic>> getComparison(String id) async {
    return _client.get<Map<String, dynamic>>(
      '/comparisons/$id',
      parse: (json) => json as Map<String, dynamic>,
    );
  }
}
