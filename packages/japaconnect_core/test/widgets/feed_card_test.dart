import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:japaconnect_core/widgets/jc_error_widget.dart';

void main() {
  testWidgets('shows retry button', (tester) async {
    var tapped = false;
    await tester.pumpWidget(MaterialApp(
      home: JcErrorWidget(error: 'oops', onRetry: () => tapped = true),
    ));
    await tester.tap(find.text('Retry'));
    expect(tapped, true);
  });
}
