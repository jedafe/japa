import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

/// Attempts to open [url] in the platform's default app.
///
/// Returns `true` if a launcher was found, `false` otherwise. On failure,
/// an optional [onError] callback is invoked with a user-friendly message.
Future<bool> jcLaunchUrl(
  BuildContext context,
  String? url, {
  VoidCallback? onCantLaunch,
}) async {
  if (url == null || url.isEmpty) return false;
  final uri = Uri.tryParse(url);
  if (uri == null) return false;

  final can = await canLaunchUrl(uri);
  if (!can) {
    onCantLaunch?.call();
    return false;
  }
  return launchUrl(uri, mode: LaunchMode.externalApplication);
}
