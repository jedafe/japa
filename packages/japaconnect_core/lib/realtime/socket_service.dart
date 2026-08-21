import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/api/client.dart';
import 'package:japaconnect_core/models/realtime_event.dart';
import 'socket_client.dart';

final socketServiceProvider = Provider<SocketService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return SocketService(client: client);
});

final realtimeEventsProvider = StreamProvider<RealtimeEvent>((ref) {
  final socket = ref.watch(socketServiceProvider);
  return socket.events;
});

class SocketService {
  final JcApiClient client;
  final _eventController = StreamController<RealtimeEvent>.broadcast();
  SocketClient? _socket;
  Timer? _reconnectTimer;

  SocketService({required this.client});

  Stream<RealtimeEvent> get events => _eventController.stream;

  Future<void> connect() async {
    if (client.accessToken == null || _socket != null) return;
    try {
      final ticket = await client.post<Map<String, dynamic>>(
        '/auth/ws-ticket',
        parse: (json) => json as Map<String, dynamic>,
      );
      _socket = SocketClient(url: client.effectiveWsUrl);
      _socket!.connect(ticket['token'] as String);
      _socket!.events.listen(
        (event) {
          if (event is Map<String, dynamic>) {
            _eventController.add(RealtimeEvent.fromJson(event));
          }
        },
        onError: (_) => _onConnectionLost(),
        onDone: _onConnectionLost,
      );
    } catch (_) {
      _socket = null;
      _scheduleReconnect();
    }
  }

  void _onConnectionLost() {
    _socket = null;
    _scheduleReconnect();
  }

  void _scheduleReconnect() {
    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(const Duration(seconds: 5), connect);
  }

  void disconnect() {
    _reconnectTimer?.cancel();
    _socket?.disconnect();
    _socket = null;
  }
}
