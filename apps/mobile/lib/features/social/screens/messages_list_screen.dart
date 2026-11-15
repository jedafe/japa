import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/social/providers/social_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class MessagesListScreen extends ConsumerWidget {
  const MessagesListScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    ref.watch(socialRealtimeSyncProvider); // keep realtime listener alive
    final state = ref.watch(conversationsProvider);
    final badges = ref.watch(verificationBadgesProvider);
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('social.messages'))),
      body: state.when(
        data: (items) => ListView.builder(
          itemCount: items.length,
          itemBuilder: (_, i) {
            final c = items[i];
            final isOrg = c.conversationType == 'org';
            final scope = isOrg ? 'organization' : 'user';
            final name = c.title ??
                c.otherDisplayName ??
                c.otherUsername ??
                c.withUser?.username ??
                t('social.conversation');
            return ListTile(
              title: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Flexible(child: Text(name, overflow: TextOverflow.ellipsis)),
                  if (c.otherVerified) ...[
                    const SizedBox(width: 4),
                    VerifiedBadge(
                      verified: c.otherVerified,
                      badgeUrl: verificationBadgeUrl(
                          badges, scope, c.otherVerificationType),
                      label: verificationBadgeLabel(
                          badges, scope, c.otherVerificationType),
                      size: 16,
                    ),
                  ],
                ],
              ),
              onTap: () => context.push('/messages/${c.id}'),
            );
          },
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
            error: e, onRetry: () => ref.invalidate(conversationsProvider)),
      ),
    );
  }
}
