import 'package:japaconnect_core/japaconnect_core.dart';

class ContentService {
  final JcApiClient _client;
  ContentService(this._client);

  Future<List<Content>> listBlog() async {
    return _client.get<List<Content>>(
      '/blog',
      parse: (json) =>
          (json as List).map((e) => Content.fromJson(e as Map<String, dynamic>)).toList(),
    );
  }

  Future<Content> getBlog(String slug) async {
    return _client.get<Content>(
      '/blog/$slug',
      parse: (json) => Content.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<List<Content>> listGuides() async {
    return _client.get<List<Content>>(
      '/content',
      query: {'type': 'guide'},
      parse: (json) =>
          (json as List).map((e) => Content.fromJson(e as Map<String, dynamic>)).toList(),
    );
  }

  Future<Content> getGuide(String slug) async {
    return _client.get<Content>(
      '/content/$slug',
      parse: (json) => Content.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<List<Content>> listComparisons() async {
    return _client.get<List<Content>>(
      '/comparisons',
      parse: (json) =>
          (json as List).map((e) => Content.fromJson(e as Map<String, dynamic>)).toList(),
    );
  }

  Future<Content> getComparison(String id) async {
    return _client.get<Content>(
      '/comparisons/$id',
      parse: (json) => Content.fromJson(json as Map<String, dynamic>),
    );
  }
}
