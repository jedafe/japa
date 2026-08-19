import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/feed/providers/trending_provider.dart';

class TrendingScreen extends ConsumerWidget {
  const TrendingScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(trendingProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Trending')),
      body: state.when(
        data: (data) => ListView(
          children: [
            _Section(title: 'Content', items: data['content']!),
            _Section(title: 'Spaces', items: data['spaces']!),
            _Section(title: 'Users', items: data['users']!),
            _Section(title: 'Tags', items: data['tags']!),
          ],
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(error: e, onRetry: () => ref.invalidate(trendingProvider)),
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
