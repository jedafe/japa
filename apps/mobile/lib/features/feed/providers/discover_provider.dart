import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/feed/services/discover_service.dart';

final discoverServiceProvider = Provider<DiscoverService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return DiscoverService(client);
});

final discoverProvider = FutureProvider.autoDispose<Map<String, List<Map<String, dynamic>>>>((ref) async {
  final service = ref.watch(discoverServiceProvider);
  final users = await service.suggestedUsers();
  final spaces = await service.suggestedSpaces();
  final tags = await service.suggestedTags();
  final content = await service.suggestedContent();
  return {'users': users, 'spaces': spaces, 'tags': tags, 'content': content};
});
