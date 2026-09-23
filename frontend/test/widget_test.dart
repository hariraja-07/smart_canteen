import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:frontend/api.dart';
import 'package:frontend/main.dart';

void main() {
  const menuJson = '''
[
  {"id":1,"name":"Pizza Margherita","category":"Main","price":9.99,"description":"Tomato, mozzarella, basil","available":true},
  {"id":2,"name":"Garlic Naan","category":"Bread","price":2.49,"description":"Oven-baked naan with garlic","available":true},
  {"id":3,"name":"Still Water","category":"Drinks","price":1.99,"description":"","available":false}
]
''';

  test('api fetchMenu parses dish list', () async {
    final client = MockClient((request) async => http.Response(menuJson, 200));
    Api.client = client;
    final dishes = await Api.fetchMenu();
    expect(dishes, hasLength(3));
    expect(dishes.first.name, 'Pizza Margherita');
    expect(dishes.first.category, 'Main');
    expect(dishes.first.price, 9.99);
    expect(dishes.last.available, isFalse);
  });

  testWidgets('shows loading then grouped menu', (WidgetTester tester) async {
    Api.client = MockClient((request) async => http.Response(menuJson, 200));

    await tester.pumpWidget(const SmartCanteenApp());
    expect(find.byType(CircularProgressIndicator), findsOneWidget);

    await tester.pumpAndSettle();
    expect(find.text('Pizza Margherita'), findsOneWidget);
    expect(find.text('Garlic Naan'), findsOneWidget);
    expect(find.text('9.99'), findsOneWidget);
    expect(find.text('Main'), findsOneWidget);
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