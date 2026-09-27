import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:frontend/api.dart';
import 'package:frontend/cart.dart';
import 'package:frontend/failure_text.dart';
import 'package:frontend/main.dart';
import 'package:frontend/models.dart';
import 'package:frontend/session.dart';

const _chai = Dish(
  id: 2,
  name: 'Masala Chai',
  category: 'Drinks',
  price: 10,
  description: '',
  available: true,
);

Session signedIn({String role = Role.canteenManagement}) {
  final s = Session();
  Api.token = 'jwt';
  s.debugSetUser(
    User(
      id: 17,
      name: 'Suresh',
      email: 'canteen@x.test',
      role: role,
      coinBalance: 0,
    ),
  );
  return s;
}

String orderJson({
  int id = 5,
  String status = OrderStatus.pending,
  int total = 70,
  String customer = 'Ravi',
}) => jsonEncode({
  'id': id,
  'user_id': 17,
  'customer': customer,
  'total': total,
  'status': status,
  'items': [
    {
      'menu_item_id': 34,
      'name': 'Masala Chai',
      'qty': 2,
      'unit_price': 10,
      'line_total': 20,
    },
  ],
  'created_at': '2026-03-04T10:15:00Z',
  'updated_at': '2026-03-04T10:15:00Z',
});

/// A queue whose contents a test controls, plus a record of every PATCH.
class Kitchen {
  List<String> statuses;
  final List<Map<String, dynamic>> patches = [];
  int? rejectWith;

  Kitchen(this.statuses);

  MockClient client() => MockClient((r) async {
    if (r.url.path == '/api/menu') return http.Response('[]', 200);
    if (r.url.path == '/api/orders' && r.method == 'GET') {
      return http.Response(
        jsonEncode([
          for (var i = 0; i < statuses.length; i++)
            {
              'id': i + 1,
              'user_id': 17,
              'customer': 'Ravi',
              'total': 70,
              'status': statuses[i],
              'items': [
                {
                  'menu_item_id': 34,
                  'name': 'Masala Chai',
                  'qty': 2,
                  'unit_price': 10,
                  'line_total': 20,
                },
              ],
              'created_at': '2026-03-04T10:15:00Z',
              'updated_at': '2026-03-04T10:15:00Z',
            },
        ]),
        200,
      );
    }
    if (r.url.path.endsWith('/status') && r.method == 'PATCH') {
      final body = jsonDecode(r.body) as Map<String, dynamic>;
      patches.add(body);
      final reject = rejectWith;
      if (reject != null) {
        return http.Response('{"error":"cannot move an order"}', reject);
      }
      // Applied to the queue, so the reload that follows a change returns the
      // new state the way a real server would. A mock that answered the same
      // stale list would make the reload untestable.
      final id = int.parse(r.url.path.split('/')[3]);
      statuses[id - 1] = body['status'] as String;
      return http.Response(
        orderJson(id: id, status: body['status'] as String),
        200,
      );
    }
    return http.Response('[]', 200);
  });
}

Future<void> openKitchen(WidgetTester tester) async {
  // Scoped to the navigation bar, because the app bar carries the same label as
  // the tab and tapping that would be ambiguous.
  await tester.tap(
    find.descendant(
      of: find.byType(NavigationBar),
      matching: find.text('Kitchen'),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  tearDown(() {
    Api.token = null;
    Api.client = http.Client();
  });

  group('who gets the kitchen tab', () {
    testWidgets('the canteen gets the kitchen tab', (tester) async {
      Api.client = Kitchen([OrderStatus.pending]).client();
      await tester.pumpWidget(
        SmartCanteenApp(session: signedIn(role: Role.canteenManagement)),
      );
      await tester.pumpAndSettle();

      expect(find.text('Kitchen'), findsWidgets);
    });

    testWidgets('the admin gets the kitchen tab', (tester) async {
      Api.client = Kitchen([OrderStatus.pending]).client();
      await tester.pumpWidget(
        SmartCanteenApp(session: signedIn(role: Role.admin)),
      );
      await tester.pumpAndSettle();

      expect(find.text('Kitchen'), findsWidgets);
    });

    testWidgets('a student is never offered it', (tester) async {
      // The tab is hidden for students, so they cannot reach a screen whose
      // every request would 403.
      Api.client = Kitchen([OrderStatus.pending]).client();
      await tester.pumpWidget(
        SmartCanteenApp(session: signedIn(role: Role.student)),
      );
      await tester.pumpAndSettle();

      expect(find.text('Kitchen'), findsNothing);
      // They still get their own orders, which the server scopes for them.
      expect(find.text('Orders'), findsWidgets);
    });
  });

  group('advancing an order', () {
    testWidgets('a pending order offers to start preparing', (tester) async {
      final kitchen = Kitchen([OrderStatus.pending]);
      Api.client = kitchen.client();
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openKitchen(tester);

      expect(find.text('Start preparing'), findsOneWidget);
      expect(find.text('Mark ready'), findsNothing);
      expect(find.text('Hand over'), findsNothing);
    });

    testWidgets('a preparing order offers to mark it ready', (tester) async {
      Api.client = Kitchen([OrderStatus.preparing]).client();
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openKitchen(tester);

      expect(find.text('Mark ready'), findsOneWidget);
      expect(find.text('Hand over'), findsNothing);
    });

    testWidgets('a ready order offers the handover', (tester) async {
      // A separate test rather than a second pumpWidget in the same body:
      // AsyncView holds its future across rebuilds, so re-pumping one tree
      // with a different mock would still be showing the first answer.
      Api.client = Kitchen([OrderStatus.ready]).client();
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openKitchen(tester);

      expect(find.text('Hand over'), findsOneWidget);
      expect(find.text('Mark ready'), findsNothing);
    });

    testWidgets('a completed order offers no action at all', (tester) async {
      Api.client = Kitchen([OrderStatus.completed]).client();
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openKitchen(tester);

      expect(find.text('Start preparing'), findsNothing);
      expect(find.text('Mark ready'), findsNothing);
      expect(find.text('Hand over'), findsNothing);
      expect(find.text('Cancel'), findsNothing);
    });

    testWidgets('a cancelled order offers no action at all', (tester) async {
      Api.client = Kitchen([OrderStatus.cancelled]).client();
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openKitchen(tester);

      expect(find.text('Start preparing'), findsNothing);
      expect(find.text('Mark ready'), findsNothing);
      expect(find.text('Hand over'), findsNothing);
      expect(find.text('Cancel'), findsNothing);
    });

    testWidgets('advancing sends the next status and reloads the queue', (
      tester,
    ) async {
      final kitchen = Kitchen([OrderStatus.pending]);
      Api.client = kitchen.client();
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openKitchen(tester);

      await tester.tap(find.text('Start preparing'));
      await tester.pumpAndSettle();

      expect(kitchen.patches.single['status'], OrderStatus.preparing);
      // Reloaded, so the row now offers the step after this one.
      expect(find.text('Mark ready'), findsOneWidget);
      expect(find.text('Start preparing'), findsNothing);
    });

    testWidgets('each row advances its own order', (tester) async {
      final kitchen = Kitchen([OrderStatus.pending, OrderStatus.preparing]);
      Api.client = kitchen.client();
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openKitchen(tester);

      // Advance the second row, which is already preparing.
      await tester.tap(find.text('Mark ready'));
      await tester.pumpAndSettle();

      expect(kitchen.patches.single['status'], OrderStatus.ready);
    });
  });

  group('cancelling', () {
    testWidgets('asks first, and cancelling is refundable', (tester) async {
      final kitchen = Kitchen([OrderStatus.preparing]);
      Api.client = kitchen.client();
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openKitchen(tester);

      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();

      // The dialog says the coins go back, so this is not one stray tap away.
      expect(find.text('Cancel this order?'), findsOneWidget);
      expect(find.textContaining('the coins returned'), findsOneWidget);

      await tester.tap(find.text('Cancel and refund'));
      await tester.pumpAndSettle();

      expect(kitchen.patches.single['status'], OrderStatus.cancelled);
    });

    testWidgets('keeping the order sends nothing', (tester) async {
      final kitchen = Kitchen([OrderStatus.preparing]);
      Api.client = kitchen.client();
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openKitchen(tester);

      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Keep it'));
      await tester.pumpAndSettle();

      expect(kitchen.patches, isEmpty);
    });
  });

  group('when the server disagrees', () {
    testWidgets('a 409 is shown and the queue is reloaded', (tester) async {
      // The order moved on since the list was fetched, so the row would keep
      // failing. Say so, then re-read the truth.
      final kitchen = Kitchen([OrderStatus.pending])..rejectWith = 409;
      Api.client = kitchen.client();
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openKitchen(tester);

      await tester.tap(find.text('Start preparing'));
      await tester.pumpAndSettle();

      expect(find.textContaining('cannot move an order'), findsOneWidget);
      // Still offered, because the reload says the order really is pending.
      expect(find.text('Start preparing'), findsOneWidget);
    });

    testWidgets('a 500 says to try again without a scary message', (
      tester,
    ) async {
      final kitchen = Kitchen([OrderStatus.pending])..rejectWith = 500;
      Api.client = kitchen.client();
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openKitchen(tester);

      await tester.tap(find.text('Start preparing'));
      await tester.pumpAndSettle();

      expect(
        // The wording comes from failure_text.dart, shared with every page.
        find.text(failureMessage(ApiException(500, 'internal error'))),
        findsOneWidget,
      );
    });

    testWidgets('an expired token sends the user back to login', (
      tester,
    ) async {
      final kitchen = Kitchen([OrderStatus.pending])..rejectWith = 401;
      Api.client = kitchen.client();
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openKitchen(tester);

      await tester.tap(find.text('Start preparing'));
      await tester.pumpAndSettle();

      expect(find.text('Sign in'), findsOneWidget);
    });
  });

  group('empty queue', () {
    testWidgets('says nothing is waiting', (tester) async {
      Api.client = Kitchen([]).client();
      await tester.pumpWidget(SmartCanteenApp(session: signedIn()));
      await tester.pumpAndSettle();
      await openKitchen(tester);

      expect(find.text('No orders waiting'), findsOneWidget);
    });
  });

  group('status helpers mirror the server', () {
    test('next follows pending, preparing, ready, completed', () {
      expect(OrderStatus.next(OrderStatus.pending), OrderStatus.preparing);
      expect(OrderStatus.next(OrderStatus.preparing), OrderStatus.ready);
      expect(OrderStatus.next(OrderStatus.ready), OrderStatus.completed);
      expect(OrderStatus.next(OrderStatus.completed), isNull);
      expect(OrderStatus.next(OrderStatus.cancelled), isNull);
    });

    test('cancelling is allowed until the order is collected', () {
      expect(OrderStatus.canCancel(OrderStatus.pending), isTrue);
      expect(OrderStatus.canCancel(OrderStatus.preparing), isTrue);
      expect(OrderStatus.canCancel(OrderStatus.ready), isTrue);
      expect(OrderStatus.canCancel(OrderStatus.completed), isFalse);
      expect(OrderStatus.canCancel(OrderStatus.cancelled), isFalse);
    });
  });

  group('the cart is per app instance', () {
    test('two carts do not share their lines', () {
      // Sharing one would let one user's dishes appear in another's cart after
      // a sign-out and sign-in as someone else.
      final first = Cart()..add(_chai);
      final second = Cart();

      expect(first.isNotEmpty, isTrue);
      expect(second.isEmpty, isTrue);

      first.clear();
      expect(second.total, 0);
    });
  });
}
