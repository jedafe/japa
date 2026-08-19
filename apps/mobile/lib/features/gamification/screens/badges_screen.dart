import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/gamification/providers/gamification_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class BadgesScreen extends ConsumerWidget {
  const BadgesScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(myBadgesProvider);
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('gamification.badges'))),
      body: state.when(
        data: (items) => ListView.builder(
          itemCount: items.length,
          itemBuilder: (_, i) => ListTile(title: Text(items[i]['name'] ?? '')),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(error: e, onRetry: () => ref.invalidate(myBadgesProvider)),
      ),
    );
  }
}
