import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:uuid/uuid.dart';

class DeviceId {
  final Future<String> value;
  DeviceId(this.value);
}

final deviceIdProvider = Provider<DeviceId>((ref) {
  return DeviceId(Future.value(const Uuid().v4()));
});
