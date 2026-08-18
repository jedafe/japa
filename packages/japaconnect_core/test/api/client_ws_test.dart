import 'package:flutter_test/flutter_test.dart';
import 'package:japaconnect_core/api/client.dart';
import 'package:japaconnect_core/auth/token_store.dart';
import 'package:japaconnect_core/auth/device_id.dart';

class FakeTokenStore extends TokenStore {
  @override
  Future<void> writeRefreshToken(String token) async {}

  @override
  Future<String?> readRefreshToken() async => null;

  @override
  Future<void> deleteRefreshToken() async {}
}

class FakeDeviceId extends DeviceId {
  @override
  Future<String> get value async => 'test-device';
}

void main() {
  test('JcApiClient.effectiveWsUrl derives ws://localhost:8080/ws from http base', () {
    final client = JcApiClient(
      baseUrl: 'http://localhost:8080/api/v1',
      tokenStore: FakeTokenStore(),
      deviceId: FakeDeviceId(),
    );
    expect(client.effectiveWsUrl, 'ws://localhost:8080/ws');
  });
}
