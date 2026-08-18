import 'dart:convert';
import 'package:web_socket_channel/web_socket_channel.dart';

class SocketClient {
  final String url;
  WebSocketChannel? _channel;

  SocketClient({required this.url});

  void connect(String ticket) {
    _channel = WebSocketChannel.connect(Uri.parse('$url?ticket=$ticket'));
  }

  Stream<dynamic> get events => _channel?.stream.map((e) => jsonDecode(e as String)) ?? const Stream.empty();

  void send(Map<String, dynamic> message) {
    _channel?.sink.add(jsonEncode(message));
  }

  void disconnect() {
    _channel?.sink.close();
    _channel = null;
  }
}
