import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/feed/services/trending_service.dart';

final trendingServiceProvider = Provider<TrendingService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return TrendingService(client);
});

final trendingProvider = FutureProvider.autoDispose<Map<String, List<Map<String, dynamic>>>>((ref) async {
  final service = ref.watch(trendingServiceProvider);
  final content = await service.trendingContent();
  final spaces = await service.trendingSpaces();
  final users = await service.trendingUsers();
  final tags = await service.trendingTags();
  return {
    'content': content.map((e) => e.toCardMap()).toList(),
    'spaces': spaces.map((e) => e.toCardMap()).toList(),
    'users': users.map((e) => e.toCardMap()).toList(),
    'tags': tags.map((e) => e.toCardMap()).toList(),
  };
});
