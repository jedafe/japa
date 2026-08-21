import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/profile/services/profile_service.dart';

final profileServiceProvider = Provider<ProfileService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return ProfileService(client);
});

final meProvider = FutureProvider.autoDispose<UserProfile>((ref) async {
  final service = ref.watch(profileServiceProvider);
  return service.getMe();
});

final preferencesProvider =
    FutureProvider.autoDispose<UserPreferences>((ref) async {
  final service = ref.watch(profileServiceProvider);
  return service.getPreferences();
});

final walletProvider = FutureProvider.autoDispose<Wallet>((ref) async {
  final service = ref.watch(profileServiceProvider);
  return service.getWallet();
});

final bookmarksProvider =
    FutureProvider.autoDispose<List<Bookmark>>((ref) async {
  final service = ref.watch(profileServiceProvider);
  return service.getBookmarks();
});
