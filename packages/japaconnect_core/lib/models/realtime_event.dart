class RealtimeEvent {
  final String type;
  final String? conversationId;
  final Map<String, dynamic>? payload;

  RealtimeEvent({
    required this.type,
    this.conversationId,
    this.payload,
  });

  factory RealtimeEvent.fromJson(Map<String, dynamic> json) {
    // Backend WSEvent envelope: { "type": "<event>", "payload": <any> } —
    // event data (e.g. conversation_id for dm_message) lives in `payload`,
    // not at the top level.
    final payload = json['payload'];
    final payloadMap = payload is Map<String, dynamic> ? payload : null;
    return RealtimeEvent(
      type: json['type'] as String? ?? 'unknown',
      conversationId: payloadMap?['conversation_id'] as String?,
      payload: payloadMap,
    );
  }

  Map<String, dynamic> toJson() => {'type': type, 'payload': payload};
}
