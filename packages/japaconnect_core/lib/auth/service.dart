import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/api/api_error.dart';
import 'package:japaconnect_core/api/client.dart';
import 'package:japaconnect_core/push/push_token_manager.dart';

final authServiceProvider = Provider<AuthService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return AuthService(client: client);
});

class AuthService {
  final JcApiClient client;

  AuthService({required this.client});

  String? get accessToken => client.accessToken;

  Future<void> login(
    String email,
    String password, {
    void Function()? onAuthenticated,
  }) async {
    final result = await client.post<Map<String, dynamic>>(
      '/auth/login',
      body: {'email': email, 'password': password},
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

  /// Restores a session from the stored refresh token on app startup.
  /// Returns `true` if a new access token was obtained.
  Future<bool> tryRestore() async {
    final refreshToken = await client.tokenStore.readRefreshToken();
    if (refreshToken == null || refreshToken.isEmpty) return false;
    return client.refresh();
  }

  Future<void> logout() async {
    try {
      final manager =
          PushTokenManager(client: client, deviceId: client.deviceId);
      await manager.deregister();
    } catch (_) {}
    await client.logout();
  }

  Future<void> _persist(Map<String, dynamic> result) async {
    final accessToken = result['access_token'] as String?;
    final refreshToken = result['refresh_token'] as String?;
    if (accessToken == null || accessToken.isEmpty) {
      throw JcApiError(
        code: 'invalid_session',
        message: 'Access token missing from response',
        status: 500,
      );
    }
    client.setAccessToken(accessToken);
    if (refreshToken != null && refreshToken.isNotEmpty) {
      await client.tokenStore.writeRefreshToken(refreshToken);
    }
  }
}
