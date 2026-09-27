import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:frontend/core/api.dart';
import 'package:frontend/core/models.dart';
import 'package:frontend/app.dart';
import 'package:frontend/core/session.dart';

Session _signedIn() {
  final session = Session(api: api);
  api.token = 'jwt';
  session.debugSetUser(
    const User(
      id: 17,
      name: 'Ravi',
      email: 'ravi@x.test',
      role: Role.student,
      coinBalance: 110,
    ),
  );
  return session;
}

/// The client the app under test talks through. Each test assigns the
/// transport it needs before pumping, so nothing is shared between them
/// and no tearDown is left over to undo one test leaking into the next.
late ApiClient api;

void main() {
  setUp(() => api = ApiClient());

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
    api = ApiClient(httpClient: client);
    final dishes = await api.fetchMenu();
    expect(dishes, hasLength(3));
    expect(dishes.first.name, 'Masala Dosa');
    expect(dishes.first.category, 'Breakfast');
    expect(dishes.first.price, 50);
    expect(dishes.last.available, isFalse);
  });

  testWidgets('shows loading then grouped menu', (WidgetTester tester) async {
    // Held open by the test rather than resolved immediately, so the loading
    // state is observed rather than raced: a mock that answers in a microtask
    // can finish before the first frame is ever drawn.
    final menu = Completer<http.Response>();
    api = ApiClient(httpClient: MockClient((request) => menu.future));

    await tester.pumpWidget(SmartCanteenApp(session: _signedIn()));
    expect(find.byType(CircularProgressIndicator), findsOneWidget);

    menu.complete(http.Response(menuJson, 200));
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
    api = ApiClient(
      httpClient: MockClient((request) async => http.Response('nope', 500)),
    );

    await tester.pumpWidget(SmartCanteenApp(session: _signedIn()));
    await tester.pumpAndSettle();

    expect(find.textContaining('Error:'), findsOneWidget);
    expect(find.text('Retry'), findsOneWidget);
  });
}
