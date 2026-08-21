import 'package:japaconnect_core/japaconnect_core.dart';

class RemittanceService {
  final JcApiClient _client;
  RemittanceService(this._client);

  Future<List<RemittanceProvider>> listProviders() async {
    final response = await _client.get<List<RemittanceProvider>>(
      '/remittance/providers',
      parse: (json) =>
          (json as List).map((e) => RemittanceProvider.fromJson(e as Map<String, dynamic>)).toList(),
    );
    return response;
  }

  Future<List<RemittanceRate>> getRates(
    String sendCurrency,
    String receiveCurrency,
  ) async {
    return _client.get<List<RemittanceRate>>(
      '/remittance/rates',
      query: {
        'send': sendCurrency,
        'receive': receiveCurrency,
      },
      parse: (json) => (json as List)
          .map((e) => RemittanceRate.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }
}
