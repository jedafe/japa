import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:japaconnect_core/japaconnect_core.dart';

import '../drafts/local_draft_store.dart';
import '../services/compose_service.dart';

final composeServiceProvider = Provider<ComposeService>((ref) {
  return ComposeService(ref.watch(jcApiClientProvider));
});

final localDraftStoreProvider = Provider<LocalDraftStore>((ref) {
  return const LocalDraftStore(FlutterSecureStorage());
});

final postableSpacesProvider = FutureProvider<List<PostableSpace>>((ref) async {
  return ref.read(composeServiceProvider).mySpaces();
});

final guideCategoriesProvider =
    FutureProvider<List<GuideCategory>>((ref) async {
  return ref.read(composeServiceProvider).guideCategories();
});

final serverDraftsProvider = FutureProvider<List<ServerDraft>>((ref) async {
  return ref.read(composeServiceProvider).serverDrafts();
});

final localDraftsProvider = FutureProvider<List<LocalDraft>>((ref) async {
  return ref.read(localDraftStoreProvider).list();
});
