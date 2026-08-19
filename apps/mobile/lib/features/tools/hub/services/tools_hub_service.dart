import 'package:japaconnect_core/japaconnect_core.dart';

class ToolsHubService {
  final JcApiClient _client;
  ToolsHubService(this._client);

  Future<Map<String, dynamic>> listFeatureFlags() async {
    return _client.get<Map<String, dynamic>>(
      '/feature-flags',
      parse: (json) => json as Map<String, dynamic>,
    );
  }
}
