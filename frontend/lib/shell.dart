import 'package:flutter/material.dart';

import 'admin_page.dart';
import 'cart.dart';
import 'cart_page.dart';
import 'coins_page.dart';
import 'kitchen_page.dart';
import 'menu_page.dart';
import 'orders_page.dart';
import 'session.dart';

/// The signed-in app. Which tabs exist is decided by the role, so a student is
/// never shown a kitchen queue they cannot read.
///
/// The tabs are driven by the session rather than by a role string typed in at
/// the call site, so a new role cannot accidentally get the wrong tabs.
class Shell extends StatefulWidget {
  const Shell({super.key});

  @override
  State<Shell> createState() => _ShellState();
}

class _ShellState extends State<Shell> {
  int _index = 0;

  /// Index of the Orders tab, jumped to after a successful order so the user
  /// lands on the confirmation they just earned.
  static const _ordersTab = 2;

  @override
  Widget build(BuildContext context) {
    final session = SessionScope.of(context);

    // Rebuilt on every role change so switching roles cannot leave a tab from
    // the previous one selected and pointing at the wrong index.
    // The cart sits between the menu and the order list, because that is the
    // order a user walks through. A badge on the tab shows the cart is not
    // empty, since a cart with coins in it is easy to forget.
    final cart = CartScope.of(context);
    final tabs = <_ShellTab>[
      const _ShellTab(icon: Icons.restaurant, label: 'Menu', page: MenuPage()),
      _ShellTab(
        icon: Icons.shopping_basket,
        label: 'Cart',
        page: CartPage(onPlaced: () => setState(() => _index = _ordersTab)),
      ),
      const _ShellTab(
        icon: Icons.receipt_long,
        label: 'Orders',
        page: OrdersPage(),
      ),
      // Only for the roles the server lets near the whole queue. A student
      // never sees this tab, so they are never offered a screen that 403s.
      if (session.canManageKitchen)
        const _ShellTab(
          icon: Icons.kitchen,
          label: 'Kitchen',
          page: KitchenPage(),
        ),
      const _ShellTab(
        icon: Icons.account_balance_wallet,
        label: 'Coins',
        page: CoinsPage(),
      ),
      // Admin alone, matching the server's rule on both routes this screen uses.
      if (session.isAdmin)
        const _ShellTab(
          icon: Icons.admin_panel_settings,
          label: 'Admin',
          page: AdminPage(),
        ),
    ];
    final safeIndex = _index.clamp(0, tabs.length - 1);

    return Scaffold(
      appBar: AppBar(
        title: Text(tabs[safeIndex].label),
        actions: [
          Center(
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12),
              child: Text(
                '${session.coinBalance} coins',
                style: Theme.of(context).textTheme.titleMedium,
              ),
            ),
          ),
          IconButton(
            tooltip: 'Sign out',
            icon: const Icon(Icons.logout),
            onPressed: session.signOut,
          ),
        ],
      ),
      body: tabs[safeIndex].page,
      // A single destination is not a navigation bar, and NavigationBar
      // rejects it outright, so the bar only appears once there is somewhere to
      // go. Hiding it also stops a one-tab shell from looking like a bug.
      bottomNavigationBar: tabs.length < 2
          ? null
          : NavigationBar(
              selectedIndex: safeIndex,
              onDestinationSelected: (i) => setState(() => _index = i),
              destinations: [
                for (final tab in tabs)
                  NavigationDestination(
                    // A dot, not a count: a badge showing "12" invites being
                    // read as a price, and it is a quantity of dishes.
                    icon: cart.count > 0 && tab.label == 'Cart'
                        ? Badge(child: Icon(tab.icon))
                        : Icon(tab.icon),
                    label: tab.label,
                  ),
              ],
            ),
    );
  }
}

class _ShellTab {
  final IconData icon;
  final String label;
  final Widget page;

  const _ShellTab({
    required this.icon,
    required this.label,
    required this.page,
  });
}
