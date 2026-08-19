import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/feed/providers/discover_provider.dart';

class DiscoverScreen extends ConsumerWidget {
  const DiscoverScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(discoverProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Discover')),
      body: state.when(
        data: (data) => ListView(
          children: [
            _Section(title: 'Users', items: data['users']!),
            _Section(title: 'Spaces', items: data['spaces']!),
            _Section(title: 'Tags', items: data['tags']!),
            _Section(title: 'Content', items: data['content']!),
          ],
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(error: e, onRetry: () => ref.invalidate(discoverProvider)),
      ),
    );
  }
}

class _Section extends StatelessWidget {
  final String title;
  final List<Map<String, dynamic>> items;
  const _Section({required this.title, required this.items});

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.all(16),
          child: Text(title, style: JcTypography.heading),
        ),
        SizedBox(
          height: 180,
          child: ListView.builder(
            scrollDirection: Axis.horizontal,
            itemCount: items.length,
            itemBuilder: (_, i) => SizedBox(
              width: 160,
              child: EntityCard(item: items[i]),
            ),
          ),
        ),
      ],
    );
  }
}
