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
    return RealtimeEvent(
      type: json['type'] as String? ?? 'unknown',
      conversationId: json['conversation_id'] as String?,
      payload: json,
    );
  }

  Map<String, dynamic> toJson() => payload ?? {'type': type};
}
