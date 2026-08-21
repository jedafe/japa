import 'package:japaconnect_core/japaconnect_core.dart';

class ToolsHubService {
  final JcApiClient _client;
  ToolsHubService(this._client);

  Future<FeatureFlags> listFeatureFlags() async {
    return _client.get<FeatureFlags>(
      '/feature-flags',
      parse: (json) => FeatureFlags.fromJson(json as Map<String, dynamic>),
    );
  }
}
