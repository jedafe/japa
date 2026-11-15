import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/api/verification_badges_provider.dart';
import 'package:japaconnect_core/widgets/avatar_widget.dart';
import 'package:japaconnect_core/widgets/verified_badge.dart';

/// Generic list-tile card for entity suggestion surfaces. Card maps may
/// carry `verified` / `verification_type` / `badge_scope` keys (§07) —
/// when `verified` is true the managed badge artwork (or generic icon
/// fallback) renders beside the title.
class EntityCard extends ConsumerWidget {
  final dynamic item;
  final VoidCallback? onTap;

  const EntityCard({super.key, required this.item, this.onTap});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final title = item['title'] ?? '';
    final subtitle = item['subtitle'] ?? '';
    final avatar = item['avatar_url'] as String?;
    final verified = item['verified'] as bool? ?? false;
    final verificationType = item['verification_type'] as String?;
    final badgeScope = item['badge_scope'] as String? ?? 'user';

    final badges = verified ? ref.watch(verificationBadgesProvider) : null;

    return ListTile(
      leading: AvatarWidget(url: avatar, size: 40),
      title: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Flexible(
            child: Text(title, overflow: TextOverflow.ellipsis),
          ),
          if (verified) ...[
            const SizedBox(width: 4),
            VerifiedBadge(
              verified: verified,
              badgeUrl: verificationBadgeUrl(
                  badges ?? const AsyncValue.data({}), badgeScope, verificationType),
              label: verificationBadgeLabel(
                  badges ?? const AsyncValue.data({}), badgeScope, verificationType),
              size: 16,
            ),
          ],
        ],
      ),
      subtitle: Text(subtitle, maxLines: 2, overflow: TextOverflow.ellipsis),
      onTap: onTap,
    );
  }
}
