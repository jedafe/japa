import 'package:japaconnect_core/japaconnect_core.dart';

class DiscoverService {
  final JcApiClient _client;
  DiscoverService(this._client);

  Future<List<User>> suggestedUsers() async {
    final response = await _client.get<List<User>>(
      '/suggestions/users',
      parse: (json) =>
          (json as List).map((e) => User.fromJson(e)!).toList(),
    );
    return response;
  }

  Future<List<Space>> suggestedSpaces() async {
    final response = await _client.get<List<Space>>(
      '/suggestions/spaces',
      parse: (json) =>
          (json as List).map((e) => Space.fromJson(e)!).toList(),
    );
    return response;
  }

  Future<List<Tag>> suggestedTags() async {
    final response = await _client.get<List<Tag>>(
      '/suggestions/tags',
      parse: (json) =>
          (json as List).map((e) => Tag.fromJson(e)!).toList(),
    );
    return response;
  }

  Future<List<Content>> suggestedContent() async {
    final response = await _client.get<List<Content>>(
      '/suggestions/content',
      parse: (json) =>
          (json as List).map((e) => Content.fromJson(e as Map<String, dynamic>)).toList(),
    );
    return response;
  }
}
