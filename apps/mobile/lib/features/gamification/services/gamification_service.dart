import 'package:japaconnect_core/japaconnect_core.dart';

class GamificationService {
  final JcApiClient _client;
  GamificationService(this._client);

  Future<List<Map<String, dynamic>>> listMyBadges() async {
    return _client.get<List<Map<String, dynamic>>>(
      '/me/badges',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
  }

  Future<List<Map<String, dynamic>>> listBadges() async {
    return _client.get<List<Map<String, dynamic>>>(
      '/gamification/badges',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
  }

  Future<List<Map<String, dynamic>>> listChallenges() async {
    return _client.get<List<Map<String, dynamic>>>(
      '/gamification/challenges',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
  }

  Future<Map<String, dynamic>> getLeaderboard({String scope = 'global', String period = 'all_time'}) async {
    return _client.get<Map<String, dynamic>>(
      '/gamification/leaderboard',
      query: {'scope': scope, 'period': period},
      parse: (json) => json as Map<String, dynamic>,
    );
  }
}
