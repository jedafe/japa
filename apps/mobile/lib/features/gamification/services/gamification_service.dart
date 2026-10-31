import 'package:japaconnect_core/japaconnect_core.dart';

class GamificationService {
  final JcApiClient _client;
  GamificationService(this._client);

  Future<List<Badge>> listMyBadges() async {
    return _client.get<List<Badge>>(
      '/me/badges',
      parse: (json) =>
          (json as List).map((e) => Badge.fromJson(e as Map<String, dynamic>)).toList(),
    );
  }

  Future<List<Badge>> listBadges() async {
    return _client.get<List<Badge>>(
      '/gamification/badges',
      parse: (json) =>
          (json as List).map((e) => Badge.fromJson(e as Map<String, dynamic>)).toList(),
    );
  }

  Future<List<Challenge>> listChallenges() async {
    return _client.get<List<Challenge>>(
      '/gamification/challenges',
      parse: (json) =>
          (json as List).map((e) => Challenge.fromJson(e as Map<String, dynamic>)).toList(),
    );
  }

  // PTS-007 — authenticated user's challenge progress.
  Future<List<Challenge>> listMyChallenges() async {
    return _client.get<List<Challenge>>(
      '/me/challenges',
      parse: (json) =>
          (json as List).map((e) => Challenge.fromJson(e as Map<String, dynamic>)).toList(),
    );
  }

  Future<Leaderboard> getLeaderboard({String scope = 'global', String period = 'all_time'}) async {
    return _client.get<Leaderboard>(
      '/gamification/leaderboard',
      query: {'scope': scope, 'period': period},
      parse: (json) => Leaderboard.fromJson(json as Map<String, dynamic>),
    );
  }
}
