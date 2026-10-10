import 'dart:convert';
import 'package:web_socket_channel/web_socket_channel.dart';

class SocketClient {
  final String url;
  WebSocketChannel? _channel;

  SocketClient({required this.url});

  void connect(String ticket) {
    // The ticket is a server-minted opaque value placed in the query
    // string (single-use, <60s TTL server-side, so access-log exposure is
    // bounded). Encode it so a future ticket format can never break the
    // URL or smuggle query parameters.
    final uri = Uri.parse(url).replace(
      queryParameters: {'ticket': ticket},
    );
    _channel = WebSocketChannel.connect(uri);
  }

  /// Parses one raw frame into a validated event envelope, or null when the
  /// frame is not a well-formed `{ "type": String, "payload"?: Map }`
  /// message. Unknown or malformed frames are dropped, never thrown —
  /// the server may add event types the client does not know yet.
  static Map<String, dynamic>? parseEvent(dynamic raw) {
    if (raw is! String) return null;
    dynamic decoded;
    try {
      decoded = jsonDecode(raw);
    } catch (_) {
      return null;
    }
    if (decoded is! Map<String, dynamic>) return null;
    final type = decoded['type'];
    if (type is! String || type.isEmpty) return null;
    final payload = decoded['payload'];
    if (payload != null && payload is! Map<String, dynamic>) return null;
    return decoded;
  }

  Stream<Map<String, dynamic>> get events =>
      _channel?.stream
          .map(parseEvent)
          .where((e) => e != null)
          .cast<Map<String, dynamic>>() ??
      const Stream.empty();

  void send(Map<String, dynamic> message) {
    _channel?.sink.add(jsonEncode(message));
  }

  void disconnect() {
    _channel?.sink.close();
    _channel = null;
  }
}
