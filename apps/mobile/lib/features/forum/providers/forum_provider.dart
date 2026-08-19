import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/forum/services/forum_service.dart';

final forumServiceProvider = Provider<ForumService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return ForumService(client);
});

final forumListProvider =
    FutureProvider.autoDispose<List<Map<String, dynamic>>>((ref) async {
  final service = ref.watch(forumServiceProvider);
  return service.listThreads();
});

final forumThreadProvider = FutureProvider.autoDispose
    .family<Map<String, dynamic>, String>((ref, slug) async {
  final service = ref.watch(forumServiceProvider);
  return service.getThread(slug);
});
