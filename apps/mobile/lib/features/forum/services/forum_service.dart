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
    final thread = await _client.get<ForumThread>(
      '/forum/$slug',
      parse: (json) => ForumThread.fromJson(json as Map<String, dynamic>),
    );
    final replies = await _client.get<List<ForumReply>>(
      '/forum/${thread.id}/replies',
      parse: (json) =>
          (json as List).map((e) => ForumReply.fromJson(e as Map<String, dynamic>)).toList(),
    );
    return thread.copyWith(replies: replies);
  }

  Future<ForumReply> postReply(String threadId, String text, {String? parentReplyId}) async {
    final body = tiptapTextToBase64(text);
    return _client.post<ForumReply>(
      '/forum/$threadId/replies',
      body: {
        'body': body,
        'parent_reply_id': parentReplyId,
      },
      parse: (json) => ForumReply.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<void> vote(String entityId, String entityType, int value) async {
    await _client.post<void>(
      '/forum/$entityId/vote',
      body: {'entity_type': entityType, 'value': value},
      parse: (_) {},
    );
  }

  Future<void> solveThread(String threadId, {String? replyId}) async {
    await _client.post<void>(
      '/forum/$threadId/solve',
      body: replyId != null ? {'reply_id': replyId} : {},
      parse: (_) {},
    );
  }

  Future<void> bookmarkThread(String threadId) async {
    await _client.post<void>(
      '/bookmarks',
      body: {'entity_type': 'thread', 'entity_id': threadId},
      parse: (_) {},
    );
  }
}
