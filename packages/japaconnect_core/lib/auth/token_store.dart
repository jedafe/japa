import 'package:flutter_secure_storage/flutter_secure_storage.dart';

const _secureStorage = FlutterSecureStorage();

class TokenStore {
  Future<void> writeRefreshToken(String token) =>
      _secureStorage.write(key: 'refresh_token', value: token);

  Future<String?> readRefreshToken() =>
      _secureStorage.read(key: 'refresh_token');

  Future<void> deleteRefreshToken() =>
      _secureStorage.delete(key: 'refresh_token');
}
