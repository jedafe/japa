import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';

/// Renders the §07 verification badge beside an entity name: the managed
/// type artwork (`badgeUrl` resolved from /verification-requests/types) when
/// present, otherwise the generic verified icon. Renders nothing when
/// `verified` is false.
class VerifiedBadge extends StatelessWidget {
  final bool verified;
  final String? badgeUrl;
  final String? label;
  final double size;

  const VerifiedBadge({
    super.key,
    required this.verified,
    this.badgeUrl,
    this.label,
    this.size = 16,
  });

  @override
  Widget build(BuildContext context) {
    if (!verified) return const SizedBox.shrink();
    final url = badgeUrl;
    if (url != null && url.isNotEmpty) {
      return Semantics(
        label: label ?? 'Verified',
        image: true,
        child: CachedNetworkImage(
          imageUrl: url,
          width: size,
          height: size,
          errorWidget: (_, __, ___) => _fallback(context),
        ),
      );
    }
    return _fallback(context);
  }

  Widget _fallback(BuildContext context) {
    return Icon(
      Icons.verified,
      size: size,
      color: Theme.of(context).colorScheme.primary,
      semanticLabel: label ?? 'Verified',
    );
  }
}
