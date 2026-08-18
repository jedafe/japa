import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:japaconnect_core/api/api_error.dart';
import 'package:japaconnect_core/api/client.dart';
import 'package:japaconnect_core/auth/device_id.dart';
import 'package:japaconnect_core/push/push_token_manager.dart';
import 'package:mocktail/mocktail.dart';

class MockJcApiClient extends Mock implements JcApiClient {}

class MockDeviceId extends Mock implements DeviceId {}

void main() {
  late MockJcApiClient client;
  late MockDeviceId deviceId;

  setUpAll(() {
    // Register fallback values for mocktail argument matchers on Map.
    registerFallbackValue(<String, dynamic>{});
    registerFallbackValue((dynamic _) {});
  });

  setUp(() {
    client = MockJcApiClient();
    deviceId = MockDeviceId();
    // Default: deviceId returns a stable value.
    when(() => deviceId.value).thenAnswer((_) async => 'test-device-id');
  });

  tearDown(() {
    debugDefaultTargetPlatformOverride = null;
  });

  group('PushTokenManager', () {
    test('can be constructed with client and deviceId', () {
      final manager = PushTokenManager(client: client, deviceId: deviceId);
      expect(manager, isNotNull);
      expect(manager.client, same(client));
      expect(manager.deviceId, same(deviceId));
    });

    group('non-mobile platform (no-op)', () {
      test('register is no-op on linux (no client calls)', () async {
        debugDefaultTargetPlatformOverride = TargetPlatform.linux;
        final manager = PushTokenManager(client: client, deviceId: deviceId);
        await manager.register('android');

        verifyNever(() => client.post<void>(
              any(),
              body: any(named: 'body'),
              parse: any(named: 'parse'),
            ));
      });

      test('deregister is no-op on linux (no client calls)', () async {
        debugDefaultTargetPlatformOverride = TargetPlatform.linux;
        final manager = PushTokenManager(client: client, deviceId: deviceId);
        await manager.deregister();

        verifyNever(() => client.delete(any()));
      });

      test('listenAndRefresh is no-op on linux', () async {
        debugDefaultTargetPlatformOverride = TargetPlatform.linux;
        final manager = PushTokenManager(client: client, deviceId: deviceId);
        await manager.listenAndRefresh();

        verifyNever(() => client.post<void>(
              any(),
              body: any(named: 'body'),
              parse: any(named: 'parse'),
            ));
        verifyNever(() => client.delete(any()));
      });
    });

    group('register (Android)', () {
      test('calls POST /me/devices with correct body on success', () async {
        debugDefaultTargetPlatformOverride = TargetPlatform.android;
        final manager = PushTokenManager(
          client: client,
          deviceId: deviceId,
          getToken: () async => 'test-token',
        );

        when(() => client.post<void>(
              '/me/devices',
              body: any(named: 'body'),
              parse: any(named: 'parse'),
            )).thenAnswer((_) async {});

        await manager.register('android');

        verify(() => client.post<void>(
              '/me/devices',
              body: {
                'device_id': 'test-device-id',
                'platform': 'android',
                'token': 'test-token',
              },
              parse: any(named: 'parse'),
            )).called(1);
      });

      test('falls back to /me/push-tokens on 404 from /me/devices', () async {
        debugDefaultTargetPlatformOverride = TargetPlatform.android;
        final manager = PushTokenManager(
          client: client,
          deviceId: deviceId,
          getToken: () async => 'test-token',
        );

        when(() => client.post<void>(
              '/me/devices',
              body: any(named: 'body'),
              parse: any(named: 'parse'),
            )).thenThrow(JcApiError(
          code: 'not_found',
          message: 'Not found',
          status: 404,
        ));
        when(() => client.post<void>(
              '/me/push-tokens',
              body: any(named: 'body'),
              parse: any(named: 'parse'),
            )).thenAnswer((_) async {});

        await manager.register('android');

        verify(() => client.post<void>(
              '/me/push-tokens',
              body: {
                'device_id': 'test-device-id',
                'platform': 'android',
                'token': 'test-token',
              },
              parse: any(named: 'parse'),
            )).called(1);
      });

      test('rethrows non-404 errors from /me/devices', () async {
        debugDefaultTargetPlatformOverride = TargetPlatform.android;
        final manager = PushTokenManager(
          client: client,
          deviceId: deviceId,
          getToken: () async => 'test-token',
        );

        when(() => client.post<void>(
              '/me/devices',
              body: any(named: 'body'),
              parse: any(named: 'parse'),
            )).thenThrow(JcApiError(
          code: 'server_error',
          message: 'Internal error',
          status: 500,
        ));

        expect(
          () => manager.register('android'),
          throwsA(isA<JcApiError>().having(
            (e) => e.status,
            'status',
            500,
          )),
        );

        // Should NOT fall back to /me/push-tokens.
        verifyNever(() => client.post<void>(
              '/me/push-tokens',
              body: any(named: 'body'),
              parse: any(named: 'parse'),
            ));
      });

      test('does nothing when token is null', () async {
        debugDefaultTargetPlatformOverride = TargetPlatform.android;
        final manager = PushTokenManager(
          client: client,
          deviceId: deviceId,
          getToken: () async => null,
        );

        await manager.register('android');

        verifyNever(() => client.post<void>(
              any(),
              body: any(named: 'body'),
              parse: any(named: 'parse'),
            ));
      });
    });

    group('register (iOS)', () {
      test('calls POST /me/devices with platform ios', () async {
        debugDefaultTargetPlatformOverride = TargetPlatform.iOS;
        final manager = PushTokenManager(
          client: client,
          deviceId: deviceId,
          getToken: () async => 'ios-token',
        );

        when(() => client.post<void>(
              '/me/devices',
              body: any(named: 'body'),
              parse: any(named: 'parse'),
            )).thenAnswer((_) async {});

        await manager.register('ios');

        verify(() => client.post<void>(
              '/me/devices',
              body: {
                'device_id': 'test-device-id',
                'platform': 'ios',
                'token': 'ios-token',
              },
              parse: any(named: 'parse'),
            )).called(1);
      });
    });

    group('deregister', () {
      test('calls DELETE /me/devices/{id} on success', () async {
        debugDefaultTargetPlatformOverride = TargetPlatform.android;
        final manager = PushTokenManager(client: client, deviceId: deviceId);

        when(() => client.delete('/me/devices/test-device-id'))
            .thenAnswer((_) async {});

        await manager.deregister();

        verify(() => client.delete('/me/devices/test-device-id')).called(1);
        verifyNever(() => client.delete('/me/push-tokens/test-device-id'));
      });

      test('falls back to DELETE /me/push-tokens/{id} on 404', () async {
        debugDefaultTargetPlatformOverride = TargetPlatform.android;
        final manager = PushTokenManager(client: client, deviceId: deviceId);

        when(() => client.delete('/me/devices/test-device-id'))
            .thenThrow(JcApiError(
          code: 'not_found',
          message: 'Not found',
          status: 404,
        ));
        when(() => client.delete('/me/push-tokens/test-device-id'))
            .thenAnswer((_) async {});

        await manager.deregister();

        verify(() => client.delete('/me/push-tokens/test-device-id')).called(1);
      });

      test('rethrows non-404 errors from /me/devices', () async {
        debugDefaultTargetPlatformOverride = TargetPlatform.android;
        final manager = PushTokenManager(client: client, deviceId: deviceId);

        when(() => client.delete('/me/devices/test-device-id'))
            .thenThrow(JcApiError(
          code: 'server_error',
          message: 'Internal error',
          status: 500,
        ));

        expect(
          () => manager.deregister(),
          throwsA(isA<JcApiError>().having(
            (e) => e.status,
            'status',
            500,
          )),
        );

        verifyNever(() => client.delete('/me/push-tokens/test-device-id'));
      });
    });
  });
}
