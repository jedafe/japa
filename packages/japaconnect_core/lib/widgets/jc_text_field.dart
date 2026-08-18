import 'package:flutter/material.dart';

class JcTextField extends StatelessWidget {
  final String label;
  final TextEditingController? controller;
  final bool obscureText;

  const JcTextField({
    super.key,
    required this.label,
    this.controller,
    this.obscureText = false,
  });

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      obscureText: obscureText,
      decoration: InputDecoration(labelText: label, border: const OutlineInputBorder()),
    );
  }
}
