import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:frontend/api.dart';
import 'package:frontend/main.dart';

void main() {
  test('api fetchHello returns trimmed body', () async {
    final client = MockClient((request) async {
      return http.Response('Hello World', 200);
    });
    Api.client = client;
    expect(await Api.fetchHello(), 'Hello World');
  });

  testWidgets('shows loading then hello world', (WidgetTester tester) async {
    Api.client = MockClient((request) async => http.Response('Hello World', 200));

    await tester.pumpWidget(const SmartCanteenApp());
    expect(find.byType(CircularProgressIndicator), findsOneWidget);

    await tester.pumpAndSettle();
    expect(find.text('Hello World'), findsOneWidget);
  });

  testWidgets('shows error and retry on failure', (WidgetTester tester) async {
    Api.client = MockClient((request) async => http.Response('nope', 500));

    await tester.pumpWidget(const SmartCanteenApp());
    await tester.pumpAndSettle();

    expect(find.textContaining('Error:'), findsOneWidget);
    expect(find.text('Retry'), findsOneWidget);
  });
}