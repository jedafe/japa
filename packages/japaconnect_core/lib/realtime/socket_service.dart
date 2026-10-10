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

  /// Bounded reconnect: 10 attempts max with exponential backoff
  /// (5s, 10s, 20s … capped at 60s), matching the web client's cap.
  /// An unbounded 5s loop would mint ws-tickets forever against a
  /// permanently-down backend.
  static const maxReconnectAttempts = 10;
  static const maxReconnectDelay = Duration(seconds: 60);
  int _reconnectAttempts = 0;

  SocketService({required this.client});

  Stream<RealtimeEvent> get events => _eventController.stream;

  Future<void> connect() async {
    if (client.accessToken == null || _socket != null) return;
    try {
      final ticket = await client.post<Map<String, dynamic>>(
        '/auth/ws-ticket',
        parse: (json) => json as Map<String, dynamic>,
      );
      // POST /auth/ws-ticket responds { "data": { "ticket": "<uuid>" } } —
      // the client unwraps `data`, so the key here is `ticket`.
      final ticketValue = ticket['ticket'];
      if (ticketValue is! String || ticketValue.isEmpty) {
        _scheduleReconnect();
        return;
      }
      _socket = SocketClient(url: client.effectiveWsUrl);
      _socket!.connect(ticketValue);
      _reconnectAttempts = 0;
      _socket!.events.listen(
        (event) {
          try {
            _eventController.add(RealtimeEvent.fromJson(event));
          } catch (_) {
            // Drop events that fail model parsing — a new server event
            // type must never crash the realtime stream.
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
    if (_reconnectAttempts >= maxReconnectAttempts) return;
    final shift = _reconnectAttempts.clamp(0, 6);
    var delay = Duration(seconds: 5 * (1 << shift));
    if (delay > maxReconnectDelay) delay = maxReconnectDelay;
    _reconnectAttempts++;
    _reconnectTimer = Timer(delay, connect);
  }

  void disconnect() {
    _reconnectTimer?.cancel();
    _reconnectAttempts = 0;
    _socket?.disconnect();
    _socket = null;
  }
}
