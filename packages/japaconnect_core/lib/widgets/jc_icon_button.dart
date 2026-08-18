import 'package:flutter/material.dart';

class JcIconButton extends StatelessWidget {
  final IconData icon;
  final VoidCallback? onPressed;

  const JcIconButton({super.key, required this.icon, this.onPressed});

  @override
  Widget build(BuildContext context) {
    return IconButton(
      icon: Icon(icon),
      onPressed: onPressed,
    );
  }
}
