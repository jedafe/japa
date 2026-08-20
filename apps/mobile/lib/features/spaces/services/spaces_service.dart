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

  Future<void> join(String slug) async {
    await _client.post<void>('/spaces/$slug/join', parse: (_) {});
  }

  Future<void> leave(String slug) async {
    await _client.delete('/spaces/$slug/leave');
  }
}
