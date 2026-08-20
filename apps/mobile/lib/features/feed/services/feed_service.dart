import 'package:japaconnect_core/japaconnect_core.dart';

class FeedService {
  final JcApiClient _client;
  FeedService(this._client);

  Future<List<Content>> fetchFeed({int page = 1}) async {
    final response = await _client.get<List<Content>>(
      '/feed',
      query: {'page': page.toString()},
      parse: (json) =>
          (json as List).map((e) => Content.fromJson(e as Map<String, dynamic>)).toList(),
    );
    return response;
  }

}
