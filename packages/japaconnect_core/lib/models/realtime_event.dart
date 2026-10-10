class RealtimeEvent {
  final String type;
  final String? conversationId;
  final Map<String, dynamic>? payload;

  /// Event types the backend emits (internal/websocket). Unknown types are
  /// rejected in fromJson so a compromised or future server can never route
  /// untrusted types into app state.
  static const _knownTypes = {
    'notification',
    'presence',
    'typing',
    'space_activity',
    'feed_update',
    'dm_message',
    'moderation_alert',
    'poll_updated',
  };

  RealtimeEvent({
    required this.type,
    this.conversationId,
    this.payload,
  });

  factory RealtimeEvent.fromJson(Map<String, dynamic> json) {
    // Backend WSEvent envelope: { "type": "<event>", "payload": <any> } —
    // event data (e.g. conversation_id for dm_message) lives in `payload`,
    // not at the top level.
    //
    // Only known event types are accepted; unknown types throw so the
    // caller drops the frame instead of routing an untrusted type into
    // app state.
    final type = json['type'];
    if (type is! String || type.isEmpty || !_knownTypes.contains(type)) {
      throw const FormatException('unknown realtime event type');
    }
    final payload = json['payload'];
    final payloadMap = payload == null
        ? null
        : payload is Map<String, dynamic>
            ? payload
            : throw const FormatException('invalid realtime event payload');
    return RealtimeEvent(
      type: type,
      conversationId: payloadMap?['conversation_id'] as String?,
      payload: payloadMap,
    );
  }

  Map<String, dynamic> toJson() => {'type': type, 'payload': payload};
}
