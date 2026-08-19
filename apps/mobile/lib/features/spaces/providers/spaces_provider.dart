import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/spaces/services/spaces_service.dart';

final spacesServiceProvider = Provider<SpacesService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return SpacesService(client);
});

final spaceListProvider =
    FutureProvider.autoDispose<List<Map<String, dynamic>>>((ref) async {
  final service = ref.watch(spacesServiceProvider);
  return service.listSpaces();
});

final spaceDetailProvider = FutureProvider.autoDispose
    .family<Map<String, dynamic>, String>((ref, slug) async {
  final service = ref.watch(spacesServiceProvider);
  return service.getSpace(slug);
});
