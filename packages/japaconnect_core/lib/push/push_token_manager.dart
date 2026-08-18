import 'package:flutter/foundation.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:japaconnect_core/auth/device_id.dart';
import 'package:japaconnect_core/api/client.dart';
import 'package:japaconnect_core/api/api_error.dart';

class PushTokenManager {
  final JcApiClient client;
  final DeviceId deviceId;

  PushTokenManager({required this.client, required this.deviceId});

  bool get _isMobile =>
      defaultTargetPlatform == TargetPlatform.android ||
      defaultTargetPlatform == TargetPlatform.iOS;

  Future<void> register(String platform) async {
    if (!_isMobile) return;
    final token = await FirebaseMessaging.instance.getToken();
    if (token == null) return;
    final id = await deviceId.value;
    final body = {'device_id': id, 'platform': platform, 'token': token};
    try {
      await client.post<void>('/me/devices', body: body, parse: (_) {});
      return;
    } on JcApiError catch (e) {
      if (e.status != 404) rethrow;
    }
    await client.post<void>(
      '/me/push-tokens',
      body: {'token': token, 'platform': platform, 'device_id': id},
      parse: (_) {},
    );
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
      final body = {'device_id': id, 'platform': platform, 'token': token};
      try {
        await client.post<void>('/me/devices', body: body, parse: (_) {});
        return;
      } on JcApiError catch (e) {
        if (e.status != 404) rethrow;
      }
      await client.post<void>(
        '/me/push-tokens',
        body: {'token': token, 'platform': platform, 'device_id': id},
        parse: (_) {},
      );
    });
  }
}
