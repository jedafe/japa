import 'package:japaconnect_core/japaconnect_core.dart';

class ProfileService {
  final JcApiClient _client;
  ProfileService(this._client);

  Future<Map<String, dynamic>> getMe() async {
    final response = await _client.get<Map<String, dynamic>>(
      '/me',
      parse: (json) => json as Map<String, dynamic>,
    );
    return response;
  }

  Future<Map<String, dynamic>> updateProfile(Map<String, dynamic> data) async {
    final response = await _client.patch<Map<String, dynamic>>(
      '/me/profile',
      body: data,
      parse: (json) => json as Map<String, dynamic>,
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

  Future<Map<String, dynamic>> getWallet() async {
    final response = await _client.get<Map<String, dynamic>>(
      '/me/wallet',
      parse: (json) => json as Map<String, dynamic>,
    );
    return response;
  }

  Future<List<Map<String, dynamic>>> getBookmarks() async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/me/bookmarks',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }
}
