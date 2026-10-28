import 'package:flutter/material.dart';

class JcTextField extends StatelessWidget {
  final String label;
  final TextEditingController? controller;
  final bool obscureText;
  final String? hint;

  const JcTextField({
    super.key,
    required this.label,
    this.controller,
    this.obscureText = false,
    this.hint,
  });

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      obscureText: obscureText,
      decoration: InputDecoration(
        labelText: label,
        border: const OutlineInputBorder(),
        suffixIcon: hint == null
            ? null
            : Tooltip(
                message: hint!,
                child: const Padding(
                  padding: EdgeInsets.all(10),
                  child: Icon(Icons.help_outline, size: 18),
                ),
              ),
      ),
    );
  }
}
