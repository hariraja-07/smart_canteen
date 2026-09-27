import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:frontend/api.dart';
import 'package:frontend/main.dart';

void main() {
  // Whole-coin prices, matching the server's CHECK constraint.
  const menuJson = '''
[
  {"id":1,"name":"Masala Dosa","category":"Breakfast","price":50,"description":"Crisp dosa, potato, chutney","available":true},
  {"id":2,"name":"Garlic Naan","category":"Bread","price":30,"description":"Oven-baked naan with garlic","available":true},
  {"id":3,"name":"Mineral Water","category":"Drinks","price":20,"description":"","available":false}
]
''';

  test('api fetchMenu parses dish list', () async {
    final client = MockClient((request) async => http.Response(menuJson, 200));
    Api.client = client;
    final dishes = await Api.fetchMenu();
    expect(dishes, hasLength(3));
    expect(dishes.first.name, 'Masala Dosa');
    expect(dishes.first.category, 'Breakfast');
    expect(dishes.first.price, 50);
    expect(dishes.last.available, isFalse);
  });

  testWidgets('shows loading then grouped menu', (WidgetTester tester) async {
    Api.client = MockClient((request) async => http.Response(menuJson, 200));

    await tester.pumpWidget(const SmartCanteenApp());
    expect(find.byType(CircularProgressIndicator), findsOneWidget);

    await tester.pumpAndSettle();
    expect(find.text('Masala Dosa'), findsOneWidget);
    expect(find.text('Garlic Naan'), findsOneWidget);
    // Whole coins, not "50.00".
    expect(find.text('50 coins'), findsOneWidget);
    expect(find.text('Breakfast'), findsOneWidget);
    expect(find.text('Bread'), findsOneWidget);
    expect(find.text('Available'), findsNWidgets(2));
    expect(find.text('Sold Out'), findsOneWidget);
  });

  testWidgets('shows error and retry on failure', (WidgetTester tester) async {
    Api.client = MockClient((request) async => http.Response('nope', 500));

    await tester.pumpWidget(const SmartCanteenApp());
    await tester.pumpAndSettle();

    expect(find.textContaining('Error:'), findsOneWidget);
    expect(find.text('Retry'), findsOneWidget);
  });
}
