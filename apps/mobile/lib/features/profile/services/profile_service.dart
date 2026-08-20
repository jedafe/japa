import 'package:japaconnect_core/japaconnect_core.dart';

class ProfileService {
  final JcApiClient _client;
  ProfileService(this._client);

  Future<UserProfile> getMe() async {
    final response = await _client.get<UserProfile>(
      '/me',
      parse: (json) => UserProfile.fromJson(json as Map<String, dynamic>),
    );
    return response;
  }

  Future<UserProfile> updateProfile(Map<String, dynamic> data) async {
    final response = await _client.patch<UserProfile>(
      '/me/profile',
      body: data,
      parse: (json) => UserProfile.fromJson(json as Map<String, dynamic>),
    );
    return response;
  }

  Future<Map<String, dynamic>> getPreferences() async {
    final response = await _client.get<Map<String, dynamic>>(
      '/me/preferences',
      parse: (json) => json as Map<String, dynamic>,
    );
    return response;
  }

  Future<Map<String, dynamic>> updatePreferences(
      Map<String, dynamic> data) async {
    final response = await _client.put<Map<String, dynamic>>(
      '/me/preferences',
      body: data,
      parse: (json) => json as Map<String, dynamic>,
    );
    return response;
  }

  Future<Wallet> getWallet() async {
    final response = await _client.get<Wallet>(
      '/me/wallet',
      parse: (json) => Wallet.fromJson(json as Map<String, dynamic>),
    );
    return response;
  }

  Future<List<Bookmark>> getBookmarks() async {
    final response = await _client.get<List<Bookmark>>(
      '/me/bookmarks',
      parse: (json) =>
          (json as List).map((e) => Bookmark.fromJson(e as Map<String, dynamic>)).toList(),
    );
    return response;
  }
}
