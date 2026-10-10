import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';

class AvatarWidget extends StatelessWidget {
  final String? url;
  final double size;

  const AvatarWidget({super.key, this.url, this.size = 40});

  /// Only https (and debug-local http) avatar URLs render. An attacker-set
  /// avatar_url like `file:///etc/hosts` or `http://attacker/pixel` must
  /// never become a local-file read or a cleartext tracking request.
  static bool _isSafeImageUrl(String? url) {
    if (url == null || url.isEmpty) return false;
    final uri = Uri.tryParse(url);
    if (uri == null || !uri.hasScheme) return false;
    if (uri.scheme == 'https') return true;
    if (kDebugMode &&
        uri.scheme == 'http' &&
        (uri.host == 'localhost' ||
            uri.host == '127.0.0.1' ||
            uri.host == '10.0.2.2')) {
      return true;
    }
    return false;
  }

  @override
  Widget build(BuildContext context) {
    return ClipOval(
      child: SizedBox(
        width: size,
        height: size,
        child: !_isSafeImageUrl(url)
            ? const ColoredBox(color: Colors.grey)
            : CachedNetworkImage(
                imageUrl: url!,
                fit: BoxFit.cover,
                placeholder: (_, __) => const ColoredBox(color: Colors.grey),
                errorWidget: (_, __, ___) => const Icon(Icons.person),
              ),
      ),
    );
  }
}
