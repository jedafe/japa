import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/api/api_error.dart';
import 'package:japaconnect_core/api/client.dart';

final authServiceProvider = Provider<AuthService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return AuthService(client: client);
});

class AuthService {
  final JcApiClient client;

  AuthService({required this.client});

  String? get accessToken => client.accessToken;

  Future<void> login(
    String username,
    String password, {
    void Function()? onAuthenticated,
  }) async {
    final result = await client.post<Map<String, dynamic>>(
      '/auth/login',
      body: {'username': username, 'password': password},
      parse: (json) => json as Map<String, dynamic>,
    );
    final challengeToken = result['challenge_token'] as String?;
    final accessToken = result['access_token'] as String?;
    if (challengeToken != null && accessToken == null) {
      throw JcApiError(
        code: 'totp_required',
        message: challengeToken,
        status: 401,
      );
    }
    await _persist(result);
    onAuthenticated?.call();
  }

  Future<void> register(
    String username,
    String email,
    String password, {
    void Function()? onAuthenticated,
  }) async {
    final result = await client.post<Map<String, dynamic>>(
      '/auth/register',
      body: {'username': username, 'email': email, 'password': password},
      parse: (json) => json as Map<String, dynamic>,
    );
    await _persist(result);
    onAuthenticated?.call();
  }

  Future<void> verifyTOTP(
    String challengeToken,
    String code, {
    void Function()? onAuthenticated,
  }) async {
    final result = await client.post<Map<String, dynamic>>(
      '/auth/totp/verify-challenge',
      body: {'challenge_token': challengeToken, 'code': code},
      parse: (json) => json as Map<String, dynamic>,
    );
    await _persist(result);
    onAuthenticated?.call();
  }

  Future<bool> refresh() async => client.refresh();

  Future<void> logout() async {
    await client.logout();
  }

  Future<void> _persist(Map<String, dynamic> result) async {
    client.setAccessToken(result['access_token'] as String);
    await client.tokenStore.writeRefreshToken(result['refresh_token'] as String);
  }
}
