import 'package:japaconnect_core/japaconnect_core.dart';

class SpacesService {
  final JcApiClient _client;
  SpacesService(this._client);

  Future<List<Space>> listSpaces() async {
    return _client.get<List<Space>>(
      '/spaces',
      parse: (json) =>
          (json as List).map((e) => Space.fromJson(e)!).toList(),
    );
  }

  Future<Space> getSpace(String slug) async {
    return _client.get<Space>(
      '/spaces/$slug',
      parse: (json) => Space.fromJson(json)!,
    );
  }

  /// Returns {joined: bool, pending: bool} — a 202 pending response for
  /// request-only spaces must not be presented as a join (BUG-035).
  Future<Map<String, bool>> join(String slug) async {
    return _client.post<Map<String, bool>>(
      '/spaces/$slug/join',
      parse: (json) => {
        'joined': (json as Map<String, dynamic>)['joined'] as bool? ?? true,
        'pending': json['pending'] as bool? ?? false,
      },
    );
  }

  Future<void> leave(String slug) async {
    await _client.delete('/spaces/$slug/leave');
  }
}
