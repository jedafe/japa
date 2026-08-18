import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:japaconnect_core/api/client.dart';
import 'package:japaconnect_core/auth/device_id.dart';
import 'push_token_manager.dart';

final pushServiceProvider = Provider<PushService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  final deviceId = ref.watch(deviceIdProvider);
  return PushService(client: client, deviceId: deviceId);
});

class PushService {
  final JcApiClient client;
  final DeviceId deviceId;
  final _foregroundController = StreamController<RemoteMessage>.broadcast();
  var _listeningToForeground = false;
  var _listeningToTokenRefresh = false;

  PushService({required this.client, required this.deviceId});

  Stream<RemoteMessage> get foregroundMessages => _foregroundController.stream;

  Future<void> initialize() async {
    if (defaultTargetPlatform != TargetPlatform.android &&
        defaultTargetPlatform != TargetPlatform.iOS) return;
    await FirebaseMessaging.instance.requestPermission();
    if (!_listeningToForeground) {
      FirebaseMessaging.onMessage
          .listen((message) => _foregroundController.add(message));
      _listeningToForeground = true;
    }
    if (client.accessToken == null) return;
    final manager = PushTokenManager(client: client, deviceId: deviceId);
    await manager.register(
        defaultTargetPlatform == TargetPlatform.iOS ? 'ios' : 'android');
    if (!_listeningToTokenRefresh) {
      manager.listenAndRefresh();
      _listeningToTokenRefresh = true;
    }
  }

  Future<void> deregister() async {
    if (defaultTargetPlatform != TargetPlatform.android &&
        defaultTargetPlatform != TargetPlatform.iOS) return;
    final manager = PushTokenManager(client: client, deviceId: deviceId);
    await manager.deregister();
  }
}
