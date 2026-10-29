import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/content/services/content_service.dart';
import 'package:japaconnect_mobile/features/content/services/reaction_service.dart';

final contentServiceProvider = Provider<ContentService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return ContentService(client);
});

final reactionServiceProvider = Provider<ReactionService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return ReactionService(client);
});

/// Combined per-content reaction state: enabled types, counts, my keys.
final contentReactionsProvider =
    FutureProvider.family<ContentReactionsState, String>((ref, contentId) async {
  final svc = ref.watch(reactionServiceProvider);
  final types = await svc.getReactionTypes();
  Map<String, int> counts = {};
  Set<String> mine = {};
  try {
    counts = await svc.getCounts(contentId);
  } catch (_) {}
  try {
    mine = await svc.getMyReactions(contentId);
  } catch (_) {}
  return ContentReactionsState(types: types, counts: counts, mine: mine);
});

class ContentReactionsState {
  final List<ReactionType> types;
  final Map<String, int> counts;
  final Set<String> mine;
  const ContentReactionsState({required this.types, required this.counts, required this.mine});
}

final blogListProvider =
    FutureProvider.autoDispose<List<Content>>((ref) async {
  final service = ref.watch(contentServiceProvider);
  return service.listBlog();
});

final blogDetailProvider = FutureProvider.autoDispose
    .family<Content, String>((ref, slug) async {
  final service = ref.watch(contentServiceProvider);
  return service.getBlog(slug);
});

final guideListProvider =
    FutureProvider.autoDispose<List<Content>>((ref) async {
  final service = ref.watch(contentServiceProvider);
  return service.listGuides();
});

final guideDetailProvider = FutureProvider.autoDispose
    .family<Content, String>((ref, slug) async {
  final service = ref.watch(contentServiceProvider);
  return service.getGuide(slug);
});

final comparisonListProvider =
    FutureProvider.autoDispose<List<Content>>((ref) async {
  final service = ref.watch(contentServiceProvider);
  return service.listComparisons();
});

final comparisonDetailProvider = FutureProvider.autoDispose
    .family<Content, String>((ref, id) async {
  final service = ref.watch(contentServiceProvider);
  return service.getComparison(id);
});
