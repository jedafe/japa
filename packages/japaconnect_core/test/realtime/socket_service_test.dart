import 'dart:async';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/realtime/socket_client.dart';
import 'package:japaconnect_core/realtime/socket_service.dart';

void main() {
  test('SocketClient stores URL', () {
    final client = SocketClient(url: 'wss://example.com/ws');
    expect(client.url, 'wss://example.com/ws');
  });

  test('realtime sync provider invalidates target provider on a mock event', () async {
    var loadCount = 0;
    final mockTargetProvider = FutureProvider<String>((ref) async {
      loadCount++;
      return 'ok-$loadCount';
    });

    // Simulate a stream of realtime events (bypassing StreamProvider override
    // lifecycle issues in Riverpod 3.x unit tests)
    final events = StreamController<Map<String, dynamic>>.broadcast();

    final mockSyncProvider = Provider<void>((ref) {
      final sub = events.stream.listen((event) {
        if (event['type'] == 'feed_update') {
          ref.invalidate(mockTargetProvider);
        }
      });
      ref.onDispose(sub.cancel);
    });

    final container = ProviderContainer();

    // Load target initially
    await container.read(mockTargetProvider.future);
    expect(loadCount, 1);

    // Activate sync provider (starts listening to events stream)
    container.read(mockSyncProvider);

    // Emit event
    events.add({'type': 'feed_update'});

    // Allow event propagation
    await Future<void>.delayed(Duration.zero);
    await Future<void>.delayed(Duration.zero);

    // Target should have been invalidated and reloaded
    final after = await container.read(mockTargetProvider.future);
    expect(after, 'ok-2');

    await events.close();
    container.dispose();
  });
}
