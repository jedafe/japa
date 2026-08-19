import 'package:japaconnect_core/japaconnect_core.dart';

class RemittanceService {
  final JcApiClient _client;
  RemittanceService(this._client);

  Future<List<Map<String, dynamic>>> listProviders() async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/remittance/providers',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }

  Future<Map<String, dynamic>> getRate(String providerId) async {
    final response = await _client.get<Map<String, dynamic>>(
      '/remittance/rates',
      query: {'provider': providerId},
      parse: (json) => json as Map<String, dynamic>,
    );
    return response;
  }
}
