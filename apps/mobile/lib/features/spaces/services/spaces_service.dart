import 'package:japaconnect_core/japaconnect_core.dart';

class SpacesService {
  final JcApiClient _client;
  SpacesService(this._client);

  Future<List<Map<String, dynamic>>> listSpaces() async {
    return _client.get<List<Map<String, dynamic>>>(
      '/spaces',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
  }

  Future<Map<String, dynamic>> getSpace(String slug) async {
    return _client.get<Map<String, dynamic>>(
      '/spaces/$slug',
      parse: (json) => json as Map<String, dynamic>,
    );
  }

  Future<void> join(String slug) async {
    await _client.post<void>('/spaces/$slug/join', parse: (_) {});
  }

  Future<void> leave(String slug) async {
    await _client.delete('/spaces/$slug/leave');
  }
}
