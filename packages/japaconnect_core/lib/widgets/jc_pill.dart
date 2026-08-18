import 'package:flutter/material.dart';

class JcPill extends StatelessWidget {
  final String label;
  final Color? color;

  const JcPill({super.key, required this.label, this.color});

  @override
  Widget build(BuildContext context) {
    return Chip(
      label: Text(label),
      backgroundColor: color,
    );
  }
}
