import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:frontend/core/api.dart';
import 'package:frontend/core/cart.dart';
import 'package:frontend/core/failure_text.dart';
import 'package:frontend/app.dart';
import 'package:frontend/core/models.dart';
import 'package:frontend/core/session.dart';

const _dosa = Dish(
  id: 1,
  name: 'Masala Dosa',
  category: 'Breakfast',
  price: 50,
  description: '',
  available: true,
);

const _chai = Dish(
  id: 2,
  name: 'Masala Chai',
  category: 'Drinks',
  price: 10,
  description: '',
  available: true,
);

const _soldOut = Dish(
  id: 3,
  name: 'Samosa',
  category: 'Snacks',
  price: 20,
  description: '',
  available: false,
);

const _menuJson =
    '[{"id":1,"name":"Masala Dosa","category":"Breakfast","price":50,"description":"","available":true},'
    '{"id":2,"name":"Masala Chai","category":"Drinks","price":10,"description":"","available":true},'
    '{"id":3,"name":"Samosa","category":"Snacks","price":20,"description":"","available":false}]';

String _meJson(int balance) =>
    '{"id":17,"name":"Ravi","email":"ravi@x.test","role":"student",'
    '"coin_balance":$balance}';

Session signedIn({int balance = 110, String role = Role.student}) {
  final s = Session(api: api);
  api.token = 'jwt';
  s.debugSetUser(
    User(
      id: 17,
      name: 'Ravi',
      email: 'ravi@x.test',
      role: role,
      coinBalance: balance,
    ),
  );
  return s;
}

/// Serves the menu, the signed-in user, and the coin history, so a test only
/// has to describe the call it actually cares about.
MockClient appRoutes(
  Future<http.Response> Function(http.Request request) handler, {
  int meBalance = 50,
}) {
  return MockClient((r) async {
    if (r.url.path == '/api/menu') return http.Response(_menuJson, 200);
    if (r.url.path == '/api/me') {
      return http.Response(_meJson(meBalance), 200);
    }
    if (r.url.path == '/api/users/17/coins') {
      return http.Response('[]', 200);
    }
    return handler(r);
  });
}

Widget app(Session session, [Cart? cart]) =>
    SmartCanteenApp(session: session, cart: cart ?? Cart());

/// Adds a dish through the menu UI, by index into the menu's Add buttons, then
/// lands on the cart tab. Only one dish is ever added this way, so the index is
/// stable: a row that gains an item swaps its Add button for a stepper.
Future<void> addAndOpenCart(WidgetTester tester, int addIndex) async {
  await tester.tap(find.text('Add').at(addIndex));
  await tester.pumpAndSettle();
  await tester.tap(find.text('Cart'));
  await tester.pumpAndSettle();
}

/// The client the app under test talks through. Each test assigns the
/// transport it needs before pumping, so nothing is shared between them
/// and no tearDown is left over to undo one test leaking into the next.
late ApiClient api;

void main() {
  setUp(() => api = ApiClient());

  group('Cart arithmetic', () {
    test('totals across lines and counts dishes', () {
      final cart = Cart()
        ..add(_dosa)
        ..add(_dosa)
        ..add(_chai);
      expect(cart.total, 110);
      expect(cart.count, 3);
      expect(cart.qtyOf(1), 2);
    });

    test('removing past zero drops the line', () {
      final cart = Cart()..add(_chai);
      cart.removeOne(2);
      expect(cart.isEmpty, isTrue);
      expect(cart.total, 0);
    });

    test('a sold-out dish cannot be added at all', () {
      // The server would reject the order, so a cart holding it is a dead end.
      final cart = Cart()..add(_soldOut);
      expect(cart.isEmpty, isTrue);
    });

    test('the request body carries ids and quantities, never prices', () {
      final cart = Cart()
        ..add(_dosa)
        ..add(_chai, 3);
      // Int-keyed on purpose: placeRequest turns this into a list of
      // string-keyed objects, and api_test asserts the encoded body has no
      // price in it.
      expect(cart.toRequest(), {1: 1, 2: 3});
    });

    test('notifies on every change that alters the cart', () {
      final cart = Cart();
      var notifications = 0;
      cart.addListener(() => notifications++);

      cart.add(_dosa);
      cart.add(_chai);
      cart.setQty(1, 5);
      cart.removeOne(2);
      cart.clear();
      // clear() on an already empty cart is a no-op and stays quiet, so a
      // rebuild is not triggered for nothing.
      cart.clear();

      expect(notifications, 5);
    });
  });

  group('adding to the cart', () {
    testWidgets('a dish in the cart shows a stepper instead of Add', (
      tester,
    ) async {
      api = ApiClient(
        httpClient: appRoutes((_) async => http.Response('[]', 200)),
      );
      final cart = Cart();
      await tester.pumpWidget(app(signedIn(), cart));
      await tester.pumpAndSettle();

      expect(find.text('Add'), findsNWidgets(2));
      await tester.tap(find.text('Add').first);
      await tester.pumpAndSettle();

      expect(find.text('Add'), findsOneWidget);
      expect(find.byTooltip('Add one'), findsOneWidget);
      expect(find.byTooltip('Remove one'), findsOneWidget);
    });

    testWidgets('a sold-out dish offers no button at all', (tester) async {
      api = ApiClient(
        httpClient: appRoutes((_) async => http.Response('[]', 200)),
      );
      await tester.pumpWidget(app(signedIn()));
      await tester.pumpAndSettle();

      // Two available dishes, so exactly two Add buttons, never a third for
      // the sold-out one.
      expect(find.text('Add'), findsNWidgets(2));
      expect(find.text('unavailable'), findsOneWidget);
    });
  });

  group('placing an order', () {
    testWidgets('sends the cart, clears it, and updates the balance', (
      tester,
    ) async {
      String? body;
      // The session is built from whatever client `api` holds, so the transport
      // has to be in place before it, or this would test a session talking to
      // the real server.
      api = ApiClient(
        httpClient: appRoutes((r) async {
          if (r.url.path == '/api/orders' && r.method == 'POST') {
            body = r.body;
            return http.Response(
              '{"id":5,"user_id":17,"customer":"Ravi","total":60,"status":"pending",'
              '"items":[],"created_at":"2026-03-04T10:15:00Z",'
              '"updated_at":"2026-03-04T10:15:00Z"}',
              201,
            );
          }
          if (r.url.path == '/api/me') {
            return http.Response(
              '{"id":17,"name":"Ravi","email":"ravi@x.test","role":"student","coin_balance":50}',
              200,
            );
          }
          return http.Response('[]', 200);
        }),
      );
      final session = signedIn(balance: 110);
      final cart = Cart();

      await tester.pumpWidget(app(session, cart));
      await tester.pumpAndSettle();
      await addAndOpenCart(tester, 0); // Masala Dosa, 50 coins

      expect(find.text('50 coins'), findsOneWidget);
      await tester.tap(find.text('Place order'));
      await tester.pumpAndSettle();

      expect(body, contains('"menu_item_id":1'));
      expect(body, contains('"qty":1'));
      expect(find.text('Order placed'), findsOneWidget);

      // Cleared, so the same order cannot be paid for twice from this cart.
      expect(cart.isEmpty, isTrue);
      // And the app bar balance is the server's new number, not the 110 the
      // cart was built against.
      expect(find.text('50 coins'), findsOneWidget);
    });

    testWidgets('the confirmation offers the order list and jumps there', (
      tester,
    ) async {
      api = ApiClient(
        httpClient: appRoutes((r) async {
          if (r.url.path == '/api/orders' && r.method == 'POST') {
            return http.Response(
              '{"id":5,"user_id":17,"customer":"Ravi","total":50,"status":"pending",'
              '"items":[],"created_at":"","updated_at":""}',
              201,
            );
          }
          return http.Response('[]', 200);
        }),
      );

      await tester.pumpWidget(app(signedIn()));
      await tester.pumpAndSettle();
      await addAndOpenCart(tester, 0);
      await tester.tap(find.text('Place order'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('View my orders'));
      await tester.pumpAndSettle();

      // Landed on Orders, not back on an empty cart.
      expect(find.text('You have not ordered yet'), findsOneWidget);
    });

    testWidgets('an empty cart cannot be submitted', (tester) async {
      api = ApiClient(
        httpClient: appRoutes((_) async => http.Response('[]', 200)),
      );
      await tester.pumpWidget(app(signedIn()));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Cart'));
      await tester.pumpAndSettle();

      expect(find.text('Your cart is empty'), findsOneWidget);
      expect(find.text('Place order'), findsNothing);
    });

    testWidgets('the button is disabled when the coins are not there', (
      tester,
    ) async {
      api = ApiClient(
        httpClient: appRoutes((_) async => http.Response('[]', 200)),
      );
      final cart = Cart()..add(_dosa); // 50 coins against a balance of 10
      await tester.pumpWidget(app(signedIn(balance: 10), cart));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Cart'));
      await tester.pumpAndSettle();

      expect(
        find.text('Not enough coins. Visit the canteen counter to top up.'),
        findsOneWidget,
      );
      final button = tester.widget<FilledButton>(
        find.ancestor(
          of: find.text('Place order'),
          matching: find.byType(FilledButton),
        ),
      );
      expect(button.onPressed, isNull);
    });

    testWidgets('a 402 from the server says what to do about it', (
      tester,
    ) async {
      // The balance can change between the button being enabled and the tap, so
      // the server can still say no. That path has to be a clear message rather
      // than a raw failure.
      api = ApiClient(
        httpClient: appRoutes((r) async {
          if (r.url.path == '/api/orders') {
            return http.Response('{"error":"not enough coins"}', 402);
          }
          return http.Response('[]', 200);
        }),
      );

      await tester.pumpWidget(app(signedIn(), Cart()..add(_chai)));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Cart'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Place order'));
      await tester.pumpAndSettle();

      expect(
        find.text('Not enough coins. Visit the canteen counter to top up.'),
        findsOneWidget,
      );
    });

    testWidgets('a sold-out dish is reported, not silently dropped', (
      tester,
    ) async {
      api = ApiClient(
        httpClient: appRoutes((r) async {
          if (r.url.path == '/api/orders') {
            return http.Response('{"error":"item 3 is sold out"}', 400);
          }
          return http.Response('[]', 200);
        }),
      );

      // The dish was available when it was added and is sold out now, which is
      // the race the server catches and the client has to report.
      final cart = Cart()..add(_chai);
      await tester.pumpWidget(app(signedIn(), cart));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Cart'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Place order'));
      await tester.pumpAndSettle();

      expect(find.textContaining('sold out'), findsOneWidget);
    });

    testWidgets('a server fault says to try again, and keeps the cart', (
      tester,
    ) async {
      final cart = Cart()..add(_chai);
      api = ApiClient(
        httpClient: appRoutes((r) async {
          if (r.url.path == '/api/orders') {
            return http.Response('{"error":"internal error"}', 500);
          }
          return http.Response('[]', 200);
        }),
      );

      await tester.pumpWidget(app(signedIn(), cart));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Cart'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Place order'));
      await tester.pumpAndSettle();

      expect(
        find.text(failureMessage(ApiException(500, 'internal error'))),
        findsOneWidget,
      );
      // Kept, because the order never happened and throwing the cart away
      // would lose what the user chose.
      expect(cart.isNotEmpty, isTrue);
    });

    testWidgets('an expired token signs the user out mid-checkout', (
      tester,
    ) async {
      // Every other screen would fail too, so the honest thing is the login
      // page rather than a cart that cannot be paid for.
      api = ApiClient(
        httpClient: appRoutes((r) async {
          if (r.url.path == '/api/orders') {
            return http.Response('{"error":"expired"}', 401);
          }
          return http.Response('[]', 200);
        }),
      );

      await tester.pumpWidget(app(signedIn(), Cart()..add(_chai)));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Cart'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Place order'));
      await tester.pumpAndSettle();

      expect(find.text('Sign in'), findsOneWidget);
      expect(api.token, isNull);
    });
  });

  group('cart tab', () {
    testWidgets('shows each line, its quantity, and the total', (tester) async {
      final cart = Cart()
        ..add(_dosa)
        ..add(_chai, 3);
      api = ApiClient(
        httpClient: appRoutes((_) async => http.Response('[]', 200)),
      );
      await tester.pumpWidget(app(signedIn(balance: 500), cart));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Cart'));
      await tester.pumpAndSettle();

      expect(find.text('Masala Dosa'), findsOneWidget);
      expect(find.text('Masala Chai'), findsOneWidget);
      expect(find.text('50 coins each'), findsOneWidget);
      expect(find.text('80 coins'), findsOneWidget);
      expect(find.text('You have 500 coins'), findsOneWidget);
    });

    testWidgets('the stepper changes the quantity and the total', (
      tester,
    ) async {
      final cart = Cart()..add(_chai);
      api = ApiClient(
        httpClient: appRoutes((_) async => http.Response('[]', 200)),
      );
      await tester.pumpWidget(app(signedIn(), cart));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Cart'));
      await tester.pumpAndSettle();

      expect(find.text('10 coins'), findsOneWidget);
      await tester.tap(find.byTooltip('Add one'));
      await tester.pumpAndSettle();

      expect(cart.qtyOf(2), 2);
      expect(find.text('20 coins'), findsOneWidget);
    });

    testWidgets('emptying the cart returns to the empty message', (
      tester,
    ) async {
      final cart = Cart()..add(_chai);
      api = ApiClient(
        httpClient: appRoutes((_) async => http.Response('[]', 200)),
      );
      await tester.pumpWidget(app(signedIn(), cart));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Cart'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Empty cart'));
      await tester.pumpAndSettle();

      expect(find.text('Your cart is empty'), findsOneWidget);
    });

    testWidgets('the cart tab carries a dot once something is in it', (
      tester,
    ) async {
      final cart = Cart();
      api = ApiClient(
        httpClient: appRoutes((_) async => http.Response('[]', 200)),
      );
      await tester.pumpWidget(app(signedIn(), cart));
      await tester.pumpAndSettle();
      expect(find.byType(Badge), findsNothing);

      cart.add(_chai);
      await tester.pumpAndSettle();
      expect(find.byType(Badge), findsOneWidget);
    });
  });
}
