import 'package:flutter/material.dart';
import 'tokens/colors.dart';

class JcButton extends StatelessWidget {
  final String label;
  final VoidCallback? onPressed;

  const JcButton({super.key, required this.label, this.onPressed});

  @override
  Widget build(BuildContext context) {
    return ElevatedButton(
      onPressed: onPressed,
      style: ElevatedButton.styleFrom(backgroundColor: JcColors.primary),
      child: Text(label),
    );
  }
}
