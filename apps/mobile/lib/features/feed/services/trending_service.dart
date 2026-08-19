import 'package:japaconnect_core/japaconnect_core.dart';

class TrendingService {
  final JcApiClient _client;
  TrendingService(this._client);

  Future<List<Map<String, dynamic>>> trendingContent() async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/trending/content',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }

  Future<List<Map<String, dynamic>>> trendingSpaces() async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/trending/spaces',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }

  Future<List<Map<String, dynamic>>> trendingUsers() async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/trending/users',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }

  Future<List<Map<String, dynamic>>> trendingTags() async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/trending/tags',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }
}
