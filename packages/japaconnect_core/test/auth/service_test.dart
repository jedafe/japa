import 'package:flutter_test/flutter_test.dart';
import 'package:japaconnect_core/api/api_error.dart';
import 'package:japaconnect_core/api/client.dart';
import 'package:japaconnect_core/auth/service.dart';
import 'package:japaconnect_core/auth/token_store.dart';
import 'package:mocktail/mocktail.dart';

class MockJcApiClient extends Mock implements JcApiClient {}

class MockTokenStore extends Mock implements TokenStore {}

void main() {
  late MockJcApiClient client;
  late MockTokenStore tokenStore;
  late AuthService service;

  setUp(() {
    client = MockJcApiClient();
    tokenStore = MockTokenStore();

    // Stub tokenStore on client
    when(() => client.tokenStore).thenReturn(tokenStore);

    // Default stubs for methods that return Future<void>
    when(() => tokenStore.writeRefreshToken(any())).thenAnswer((_) async {});
    when(() => client.setAccessToken(any())).thenReturn(null);

    service = AuthService(client: client);
  });

  test('stores access token and refresh token on login', () async {
    when(() => client.post<Map<String, dynamic>>(
          '/auth/login',
          body: any(named: 'body'),
          parse: any(named: 'parse'),
        )).thenAnswer((_) async => {
          'access_token': 'access-123',
          'refresh_token': 'refresh-456',
        });

    await service.login('user', 'pass');

    verify(() => client.setAccessToken('access-123')).called(1);
    verify(() => tokenStore.writeRefreshToken('refresh-456')).called(1);
  });

  test('stores tokens and calls onAuthenticated on register', () async {
    var called = false;
    when(() => client.post<Map<String, dynamic>>(
          '/auth/register',
          body: any(named: 'body'),
          parse: any(named: 'parse'),
        )).thenAnswer((_) async => {
          'access_token': 'reg-access',
          'refresh_token': 'reg-refresh',
        });

    await service.register('user', 'e@m.com', 'pass',
        onAuthenticated: () => called = true);

    verify(() => client.setAccessToken('reg-access')).called(1);
    verify(() => tokenStore.writeRefreshToken('reg-refresh')).called(1);
    expect(called, isTrue);
  });

  test('throws JcApiError with code totp_required when challenge token present',
      () async {
    when(() => client.post<Map<String, dynamic>>(
          '/auth/login',
          body: any(named: 'body'),
          parse: any(named: 'parse'),
        )).thenAnswer((_) async => {
          'challenge_token': 'challenge-xyz',
        });

    expect(
      () => service.login('user', 'pass'),
      throwsA(isA<JcApiError>()
          .having((e) => e.code, 'code', 'totp_required')
          .having((e) => e.message, 'message', 'challenge-xyz')),
    );

    verifyNever(() => client.setAccessToken(any()));
    verifyNever(() => tokenStore.writeRefreshToken(any()));
  });

  test('verifyTOTP stores tokens on success', () async {
    when(() => client.post<Map<String, dynamic>>(
          '/auth/totp/verify-challenge',
          body: any(named: 'body'),
          parse: any(named: 'parse'),
        )).thenAnswer((_) async => {
          'access_token': 'totp-access',
          'refresh_token': 'totp-refresh',
        });

    await service.verifyTOTP('challenge-xyz', '123456');

    verify(() => client.setAccessToken('totp-access')).called(1);
    verify(() => tokenStore.writeRefreshToken('totp-refresh')).called(1);
  });

  test('logout delegates to client.logout', () async {
    when(() => client.logout()).thenAnswer((_) async {});

    await service.logout();

    verify(() => client.logout()).called(1);
  });

  test('refresh delegates to client.refresh', () async {
    when(() => client.refresh()).thenAnswer((_) async => true);

    final result = await service.refresh();

    expect(result, isTrue);
    verify(() => client.refresh()).called(1);
  });

  test('accessToken getter delegates to client', () {
    when(() => client.accessToken).thenReturn('tok-123');

    expect(service.accessToken, 'tok-123');
  });
}
