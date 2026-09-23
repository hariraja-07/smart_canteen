import 'package:flutter_test/flutter_test.dart';

import 'package:frontend/main.dart';

void main() {
  testWidgets('shows hello world', (WidgetTester tester) async {
    await tester.pumpWidget(const SmartCanteenApp());

    expect(find.text('Hello World'), findsOneWidget);
  });
}