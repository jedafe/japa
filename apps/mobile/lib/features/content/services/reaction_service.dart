import 'package:japaconnect_core/japaconnect_core.dart';

/// Reaction type from GET /reaction-types (admin-configured, §14).
class ReactionType {
  final String key;
  final String emoji;
  final String label;
  const ReactionType({required this.key, required this.emoji, required this.label});

  factory ReactionType.fromJson(Map<String, dynamic> json) => ReactionType(
        key: json['key'] as String,
        emoji: json['emoji'] as String? ?? '',
        label: json['label'] as String? ?? '',
      );
}

/// One reactor row from GET /content/{id}/reactors ("who reacted").
class Reactor {
  final String userId;
  final String username;
  final String displayName;
  final String reactionKey;
  const Reactor({
    required this.userId,
    required this.username,
    required this.displayName,
    required this.reactionKey,
  });

  factory Reactor.fromJson(Map<String, dynamic> json) => Reactor(
        userId: json['user_id'] as String,
        username: json['username'] as String? ?? '',
        displayName: json['display_name'] as String? ?? '',
        reactionKey: json['reaction_key'] as String? ?? '',
      );
}

/// Emoji reactions on content (§14) — parity with the web ReactionRail.
/// Add/remove are idempotent server-side; counts come from the per-key
/// aggregate endpoint.
class ReactionService {
  final JcApiClient _client;
  ReactionService(this._client);

  Future<List<ReactionType>> getReactionTypes() async {
    return _client.get<List<ReactionType>>(
      '/reaction-types',
      parse: (json) => (json as List)
          .map((e) => ReactionType.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }

  Future<Map<String, int>> getCounts(String contentId) async {
    return _client.get<Map<String, int>>(
      '/content/$contentId/reactions',
      parse: (json) => {
        for (final e in (json as List))
          (e as Map<String, dynamic>)['reaction_key'] as String:
              (e['count'] as num).toInt(),
      },
    );
  }

  Future<Set<String>> getMyReactions(String contentId) async {
    return _client.get<Set<String>>(
      '/content/$contentId/reactions/me',
      parse: (json) =>
          (json as List).map((e) => (e as Map<String, dynamic>)['reaction_key'] as String).toSet(),
    );
  }

  Future<void> react(String contentId, String reactionKey) async {
    await _client.post<void>(
      '/content/$contentId/reactions',
      body: {'reaction_key': reactionKey},
      parse: (_) {},
    );
  }

  Future<void> unreact(String contentId, String reactionKey) async {
    await _client.delete(
      '/content/$contentId/reactions/${Uri.encodeComponent(reactionKey)}',
    );
  }

  Future<List<Reactor>> getReactors(String contentId, {String? reactionKey}) async {
    return _client.get<List<Reactor>>(
      '/content/$contentId/reactors',
      query: {
        if (reactionKey != null) 'key': reactionKey,
        'page_size': '25',
      },
      parse: (json) =>
          (json as List).map((e) => Reactor.fromJson(e as Map<String, dynamic>)).toList(),
    );
  }
}
