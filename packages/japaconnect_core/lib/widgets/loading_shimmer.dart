import 'package:flutter/material.dart';

class LoadingShimmer extends StatelessWidget {
  const LoadingShimmer({super.key});

  @override
  Widget build(BuildContext context) {
    return ListView.builder(
      itemCount: 6,
      itemBuilder: (_, __) => const Padding(
        padding: EdgeInsets.all(16),
        child: SizedBox(height: 80, child: ColoredBox(color: Colors.black12)),
      ),
    );
  }
}
