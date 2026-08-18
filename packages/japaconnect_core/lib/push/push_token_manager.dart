import 'package:flutter/foundation.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:japaconnect_core/auth/device_id.dart';
import 'package:japaconnect_core/api/client.dart';
import 'package:japaconnect_core/api/api_error.dart';

class PushTokenManager {
  final JcApiClient client;
  final DeviceId deviceId;
  final Future<String?> Function()? _getToken;

  PushTokenManager({
    required this.client,
    required this.deviceId,
    Future<String?> Function()? getToken,
  }) : _getToken = getToken;

  bool get _isMobile =>
      defaultTargetPlatform == TargetPlatform.android ||
      defaultTargetPlatform == TargetPlatform.iOS;

  Future<String?> _resolveToken() async {
    final getToken = _getToken;
    if (getToken != null) return getToken();
    return FirebaseMessaging.instance.getToken();
  }

  /// Registers a token with the primary endpoint, falling back to the
  /// legacy endpoint if the primary returns 404.
  Future<void> _registerToken({
    required String id,
    required String platform,
    required String token,
  }) async {
    final body = {'device_id': id, 'platform': platform, 'token': token};
    try {
      await client.post<void>('/me/devices', body: body, parse: (_) {});
      return;
    } on JcApiError catch (e) {
      if (e.status != 404) rethrow;
    }
    await client.post<void>(
      '/me/push-tokens',
      body: body,
      parse: (_) {},
    );
  }

  Future<void> register(String platform) async {
    if (!_isMobile) return;
    final token = await _resolveToken();
    if (token == null) return;
    final id = await deviceId.value;
    await _registerToken(id: id, platform: platform, token: token);
  }

  Future<void> deregister() async {
    if (!_isMobile) return;
    final id = await deviceId.value;
    try {
      await client.delete('/me/devices/$id');
      return;
    } on JcApiError catch (e) {
      if (e.status != 404) rethrow;
    }
    await client.delete('/me/push-tokens/$id');
  }

  Future<void> listenAndRefresh() async {
    if (!_isMobile) return;
    FirebaseMessaging.instance.onTokenRefresh.listen((token) async {
      if (client.accessToken == null) return;
      final id = await deviceId.value;
      final platform =
          defaultTargetPlatform == TargetPlatform.iOS ? 'ios' : 'android';
      await _registerToken(id: id, platform: platform, token: token);
    });
  }
}
