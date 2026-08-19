import 'package:japaconnect_core/japaconnect_core.dart';

class FeedService {
  final JcApiClient _client;
  FeedService(this._client);

  Future<List<Map<String, dynamic>>> fetchFeed({int page = 1}) async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/feed',
      query: {'page': page.toString()},
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }

  Future<List<Map<String, dynamic>>> fetchForYou({int page = 1}) async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/feed/for-you',
      query: {'page': page.toString()},
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }
}
