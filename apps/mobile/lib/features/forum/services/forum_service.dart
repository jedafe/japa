import 'package:japaconnect_core/japaconnect_core.dart';

class ForumService {
  final JcApiClient _client;
  ForumService(this._client);

  Future<List<Map<String, dynamic>>> listThreads() async {
    return _client.get<List<Map<String, dynamic>>>(
      '/forum',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
  }

  Future<Map<String, dynamic>> getThread(String slug) async {
    return _client.get<Map<String, dynamic>>(
      '/forum/$slug',
      parse: (json) => json as Map<String, dynamic>,
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
