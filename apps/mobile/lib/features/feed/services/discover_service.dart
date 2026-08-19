import 'package:japaconnect_core/japaconnect_core.dart';

class DiscoverService {
  final JcApiClient _client;
  DiscoverService(this._client);

  Future<List<Map<String, dynamic>>> suggestedUsers() async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/suggestions/users',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }

  Future<List<Map<String, dynamic>>> suggestedSpaces() async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/suggestions/spaces',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }

  Future<List<Map<String, dynamic>>> suggestedTags() async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/suggestions/tags',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }

  Future<List<Map<String, dynamic>>> suggestedContent() async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/suggestions/content',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }
}
