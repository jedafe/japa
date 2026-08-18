import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:japaconnect_core/api/client.dart';
import 'package:japaconnect_core/auth/device_id.dart';
import 'package:japaconnect_core/push/push_token_manager.dart';
import 'package:mocktail/mocktail.dart';

class MockJcApiClient extends Mock implements JcApiClient {}

class MockDeviceId extends Mock implements DeviceId {}

void main() {
  late MockJcApiClient client;
  late MockDeviceId deviceId;
  late PushTokenManager manager;

  setUp(() {
    client = MockJcApiClient();
    deviceId = MockDeviceId();
    manager = PushTokenManager(client: client, deviceId: deviceId);
  });

  tearDown(() {
    debugDefaultTargetPlatformOverride = null;
  });

  group('PushTokenManager', () {
    test('can be constructed with client and deviceId', () {
      expect(manager, isNotNull);
      expect(manager.client, same(client));
      expect(manager.deviceId, same(deviceId));
    });

    test('register is no-op on non-mobile platform (no client calls)',
        () async {
      debugDefaultTargetPlatformOverride = TargetPlatform.linux;
      await manager.register('android');

      verifyNever(() => client.post<void>(
            any(),
            body: any(named: 'body'),
            parse: any(named: 'parse'),
          ));
    });

    test('deregister is no-op on non-mobile platform (no client calls)',
        () async {
      debugDefaultTargetPlatformOverride = TargetPlatform.linux;
      await manager.deregister();

      verifyNever(() => client.delete(any()));
    });

    test('listenAndRefresh is no-op on non-mobile platform', () async {
      debugDefaultTargetPlatformOverride = TargetPlatform.linux;
      await manager.listenAndRefresh();

      verifyNever(() => client.post<void>(
            any(),
            body: any(named: 'body'),
            parse: any(named: 'parse'),
          ));
      verifyNever(() => client.delete(any()));
    });
  });
}
