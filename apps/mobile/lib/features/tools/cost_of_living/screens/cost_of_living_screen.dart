import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/cost_of_living/providers/cost_of_living_provider.dart';

final _costOfLivingInputsProvider = NotifierProvider<
    CostOfLivingInputsNotifier,
    ({String origin, String destination})>(
  () => CostOfLivingInputsNotifier(),
);

class CostOfLivingInputsNotifier
    extends Notifier<({String origin, String destination})> {
  @override
  ({String origin, String destination}) build() =>
      (origin: '', destination: '');

  void updateInputs(String origin, String destination) {
    state = (origin: origin, destination: destination);
  }
}

class CostOfLivingScreen extends ConsumerWidget {
  const CostOfLivingScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final inputs = ref.watch(_costOfLivingInputsProvider);
    final state = ref.watch(costOfLivingProvider(inputs));
    final origin = TextEditingController();
    final destination = TextEditingController();

    Widget buildForm() {
      return Column(
        children: [
          JcTextField(label: 'Origin', controller: origin),
          JcTextField(label: 'Destination', controller: destination),
          JcButton(
            label: 'Compare',
            onPressed: () {
              ref
                  .read(_costOfLivingInputsProvider.notifier)
                  .updateInputs(origin.text, destination.text);
            },
          ),
        ],
      );
    }

    return Scaffold(
      appBar: AppBar(title: const Text('Cost of Living')),
      body: state.when(
        data: (result) {
          final items = result['items'] as List? ?? [];
          return ListView(
            children: [
              buildForm(),
              ...items.map<Widget>((item) => ListTile(
                    title: Text(item['label'] as String? ?? ''),
                    trailing: Text(item['value']?.toString() ?? ''),
                  )),
            ],
          );
        },
        loading: () =>
            inputs.origin.isEmpty ? buildForm() : const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(costOfLivingProvider(inputs)),
        ),
      ),
    );
  }
}
