import 'package:japaconnect_core/japaconnect_core.dart';

class TrendingService {
  final JcApiClient _client;
  TrendingService(this._client);

  Future<List<Content>> trendingContent() async {
    final response = await _client.get<List<Content>>(
      '/trending/content',
      parse: (json) =>
          (json as List).map((e) => Content.fromJson(e as Map<String, dynamic>)).toList(),
    );
    return response;
  }

  Future<List<Space>> trendingSpaces() async {
    final response = await _client.get<List<Space>>(
      '/trending/spaces',
      parse: (json) =>
          (json as List).map((e) => Space.fromJson(e)!).toList(),
    );
    return response;
  }

  Future<List<User>> trendingUsers() async {
    final response = await _client.get<List<User>>(
      '/trending/users',
      parse: (json) =>
          (json as List).map((e) => User.fromJson(e)!).toList(),
    );
    return response;
  }

  Future<List<Tag>> trendingTags() async {
    final response = await _client.get<List<Tag>>(
      '/trending/tags',
      parse: (json) =>
          (json as List).map((e) => Tag.fromJson(e)!).toList(),
    );
    return response;
  }
}
