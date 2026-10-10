import 'package:flutter_test/flutter_test.dart';
import 'package:japaconnect_core/models/realtime_event.dart';
import 'package:japaconnect_core/realtime/socket_client.dart';

void main() {
  group('SocketClient.parseEvent', () {
    test('accepts a well-formed envelope', () {
      final event = SocketClient.parseEvent(
        '{"type":"dm_message","payload":{"conversation_id":"abc"}}',
      );
      expect(event, isNotNull);
      expect(event!['type'], 'dm_message');
    });

    test('rejects non-string frames', () {
      expect(SocketClient.parseEvent(42), isNull);
      expect(SocketClient.parseEvent({'type': 'x'}), isNull);
    });

    test('rejects malformed JSON', () {
      expect(SocketClient.parseEvent('not json {'), isNull);
    });

    test('rejects missing or empty type', () {
      expect(SocketClient.parseEvent('{"payload":{}}'), isNull);
      expect(SocketClient.parseEvent('{"type":""}'), isNull);
      expect(SocketClient.parseEvent('{"type":42}'), isNull);
    });

    test('rejects non-map payload', () {
      expect(
        SocketClient.parseEvent('{"type":"dm_message","payload":"x"}'),
        isNull,
      );
    });

    test('accepts missing payload', () {
      final event = SocketClient.parseEvent('{"type":"presence"}');
      expect(event, isNotNull);
      expect(event!['type'], 'presence');
    });
  });

  group('RealtimeEvent.fromJson', () {
    test('parses a known type with payload', () {
      final event = RealtimeEvent.fromJson({
        'type': 'dm_message',
        'payload': {'conversation_id': 'c1'},
      });
      expect(event.type, 'dm_message');
      expect(event.conversationId, 'c1');
    });

    test('throws on unknown type', () {
      expect(
        () => RealtimeEvent.fromJson({'type': 'evil_inject'}),
        throwsFormatException,
      );
    });

    test('throws on legacy unknown fallback', () {
      expect(
        () => RealtimeEvent.fromJson({'type': 'unknown'}),
        throwsFormatException,
      );
    });

    test('throws on non-map payload', () {
      expect(
        () => RealtimeEvent.fromJson(
            {'type': 'dm_message', 'payload': 'x'}),
        throwsFormatException,
      );
    });
  });
}
