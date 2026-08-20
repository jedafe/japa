import 'package:japaconnect_core/japaconnect_core.dart';

class ForumService {
  final JcApiClient _client;
  ForumService(this._client);

  Future<List<ForumThread>> listThreads() async {
    return _client.get<List<ForumThread>>(
      '/forum',
      parse: (json) =>
          (json as List).map((e) => ForumThread.fromJson(e as Map<String, dynamic>)).toList(),
    );
  }

  Future<ForumThread> getThread(String slug) async {
    return _client.get<ForumThread>(
      '/forum/$slug',
      parse: (json) => ForumThread.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<void> postReply(String threadId, String body) async {
    await _client.post<void>(
      '/forum/$threadId/replies',
      body: {'body': body},
      parse: (_) {},
    );
  }
}
