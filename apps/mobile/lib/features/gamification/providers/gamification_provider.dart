import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/gamification/services/gamification_service.dart';

final gamificationServiceProvider = Provider<GamificationService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return GamificationService(client);
});

final myBadgesProvider = FutureProvider.autoDispose<List<Badge>>((ref) async {
  final service = ref.watch(gamificationServiceProvider);
  return service.listMyBadges();
});

final leaderboardProvider = FutureProvider.autoDispose<Leaderboard>((ref) async {
  final service = ref.watch(gamificationServiceProvider);
  return service.getLeaderboard();
});
