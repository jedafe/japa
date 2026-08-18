import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:uuid/uuid.dart';

class DeviceId {
  static const _storageKey = 'device_id';
  final FlutterSecureStorage _storage;

  DeviceId({FlutterSecureStorage? storage})
      : _storage = storage ?? const FlutterSecureStorage();

  Future<String> get value async {
    final stored = await _storage.read(key: _storageKey);
    if (stored != null) return stored;
    final id = const Uuid().v4();
    await _storage.write(key: _storageKey, value: id);
    return id;
  }
}

final deviceIdProvider = Provider<DeviceId>((ref) {
  return DeviceId();
});
