import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/spaces/providers/spaces_provider.dart';


class SpaceDetailScreen extends ConsumerWidget {
  final String slug;
  const SpaceDetailScreen({super.key, required this.slug});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(spaceDetailProvider(slug));
    return Scaffold(
      appBar: AppBar(title: const Text('Space')),
      body: state.when(
        data: (space) => Column(
          children: [
            ListTile(title: Text(space['name'] ?? '')),
            Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                JcButton(
                  label: 'Join',
                  onPressed: () => ref.read(spacesServiceProvider).join(slug),
                ),
                const SizedBox(width: 16),
                JcButton(
                  label: 'Leave',
                  onPressed: () => ref.read(spacesServiceProvider).leave(slug),
                ),
              ],
            ),
          ],
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(spaceDetailProvider(slug)),
        ),
      ),
    );
  }
}
