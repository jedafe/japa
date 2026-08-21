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

  Future<UserPreferences> getPreferences() async {
    final response = await _client.get<UserPreferences>(
      '/me/preferences',
      parse: (json) => UserPreferences.fromJson(json as Map<String, dynamic>),
    );
    return response;
  }

  Future<UserPreferences> updatePreferences(UserPreferences preferences) async {
    final response = await _client.put<UserPreferences>(
      '/me/preferences',
      body: preferences.toJson(),
      parse: (json) => UserPreferences.fromJson(json as Map<String, dynamic>),
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
