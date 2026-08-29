import 'dart:convert';

/// Encode plain text as a base64 TipTap/ProseMirror JSON document.
/// The forum backend expects `body` as a base64-encoded JSON document
/// (Section 14 / Section 08).
String tiptapTextToBase64(String text) {
  final doc = {
    'type': 'doc',
    'content': [
      {
        'type': 'paragraph',
        'content': [
          {'type': 'text', 'text': text}
        ]
      }
    ]
  };
  final json = jsonEncode(doc);
  return base64Encode(utf8.encode(json));
}

/// Decode a base64 TipTap/ProseMirror JSON document and extract the plain text.
/// Returns the original string if decoding fails (e.g. the body is already plain text).
String tiptapBase64ToText(String? base64Body) {
  if (base64Body == null || base64Body.isEmpty) return '';
  try {
    final bytes = base64Decode(base64Body);
    final json = utf8.decode(bytes);
    final doc = jsonDecode(json) as Map<String, dynamic>;
    return _extractText(doc);
  } catch (_) {
    return base64Body;
  }
}

String _extractText(Map<String, dynamic> node) {
  final buffer = StringBuffer();
  final text = node['text'];
  if (text is String) {
    buffer.write(text);
    buffer.write(' ');
  }
  final content = node['content'];
  if (content is List) {
    for (final child in content) {
      if (child is Map<String, dynamic>) {
        buffer.write(_extractText(child));
      }
    }
  }
  return buffer.toString().trim();
}
