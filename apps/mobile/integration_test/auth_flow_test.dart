import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:japaconnect_mobile/main.dart' as app;

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('login and reach feed', (tester) async {
    app.main();
    await tester.pumpAndSettle();
    expect(find.text('Login'), findsOneWidget);
  });
}
