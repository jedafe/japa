import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/content/services/content_service.dart';

final contentServiceProvider = Provider<ContentService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return ContentService(client);
});

final blogListProvider =
    FutureProvider.autoDispose<List<Map<String, dynamic>>>((ref) async {
  final service = ref.watch(contentServiceProvider);
  return service.listBlog();
});

final blogDetailProvider = FutureProvider.autoDispose
    .family<Map<String, dynamic>, String>((ref, slug) async {
  final service = ref.watch(contentServiceProvider);
  return service.getBlog(slug);
});

final guideListProvider =
    FutureProvider.autoDispose<List<Map<String, dynamic>>>((ref) async {
  final service = ref.watch(contentServiceProvider);
  return service.listGuides();
});

final guideDetailProvider = FutureProvider.autoDispose
    .family<Map<String, dynamic>, String>((ref, slug) async {
  final service = ref.watch(contentServiceProvider);
  return service.getGuide(slug);
});

final comparisonListProvider =
    FutureProvider.autoDispose<List<Map<String, dynamic>>>((ref) async {
  final service = ref.watch(contentServiceProvider);
  return service.listComparisons();
});

final comparisonDetailProvider = FutureProvider.autoDispose
    .family<Map<String, dynamic>, String>((ref, id) async {
  final service = ref.watch(contentServiceProvider);
  return service.getComparison(id);
});
