import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

/// Attempts to open [url] in the platform's default app.
///
/// Only http/https links (plus mailto:/tel: for contact actions) launch.
/// Arbitrary schemes (`intent:`, `file:`, `javascript:`, custom app schemes)
/// from attacker-influenced content (article links, bios) must never fire
/// external-app intents. Returns `true` if a launcher was found, `false`
/// otherwise. On failure, an optional [onError] callback is invoked with a
/// user-friendly message.
Future<bool> jcLaunchUrl(
  BuildContext context,
  String? url, {
  VoidCallback? onCantLaunch,
}) async {
  if (url == null || url.isEmpty) return false;
  final uri = Uri.tryParse(url.trim());
  if (uri == null || !uri.hasScheme) return false;

  const allowed = {'https', 'http', 'mailto', 'tel'};
  if (!allowed.contains(uri.scheme.toLowerCase())) return false;

  final can = await canLaunchUrl(uri);
  if (!can) {
    onCantLaunch?.call();
    return false;
  }
  return launchUrl(uri, mode: LaunchMode.externalApplication);
}
