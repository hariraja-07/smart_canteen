import 'package:flutter/material.dart';

import 'api.dart';
import 'async_action.dart';
import 'async_view.dart';
import 'models.dart';
import 'orders_page.dart';
import 'session.dart';

/// The kitchen queue: every order, with the one step each is waiting for.
///
/// Each row offers only the transition the server will accept from its current
/// state, so a screen full of buttons that all fail is never rendered. The
/// server is still the authority, and a 409 is reported rather than hidden.
class KitchenPage extends StatefulWidget {
  const KitchenPage({super.key});

  @override
  State<KitchenPage> createState() => _KitchenPageState();
}

class _KitchenPageState extends State<KitchenPage> {
  /// Bumped after a successful change to reload the queue.
  int _version = 0;

  /// The order currently being changed, or 0. Order ids start at 1, so 0 is a
  /// safe "nothing in flight" marker without a nullable field.
  int _busyOrder = 0;

  Future<void> _advance(Order order) async {
    final next = OrderStatus.next(order.status);
    if (next == null) return;
    await _apply(order.id, next);
  }

  Future<void> _apply(int orderId, String status) async {
    setState(() => _busyOrder = orderId);
    try {
      // The queue is reloaded whether this succeeded or failed. A 409 means the
      // order moved on since this list was fetched, and leaving the stale row in
      // place would leave a button that keeps failing.
      await runMutation(context, () => Api.setOrderStatus(orderId, status));
      if (mounted) setState(() => _version++);
    } finally {
      if (mounted) setState(() => _busyOrder = 0);
    }
  }

  /// Cancelling refunds the customer, so it asks first. A tap that gives the
  /// money back should not be one stray tap away.
  Future<void> _confirmCancel(Order order) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Cancel this order?'),
        content: Text(
          'Order #${order.id} for ${order.total} coins will be cancelled and '
          'the coins returned to ${order.customer}.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Keep it'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Cancel and refund'),
          ),
        ],
      ),
    );
    if (confirmed == true && mounted) {
      await _apply(order.id, OrderStatus.cancelled);
    }
  }

  @override
  Widget build(BuildContext context) {
    final session = SessionScope.of(context);
    if (!session.canManageKitchen) {
      // Unreachable through the tabs; kept so this screen is not a way to see
      // the whole queue by being mounted somewhere unexpected.
      return const Center(child: Text('Not available for your role'));
    }

    return AsyncView<List<Order>>(
      load: Api.fetchOrders,
      refreshable: true,
      refreshToken: _version,
      isEmpty: (orders) => orders.isEmpty,
      emptyMessage: 'No orders waiting',
      builder: (context, orders) => ListView.separated(
        padding: const EdgeInsets.all(12),
        itemCount: orders.length,
        separatorBuilder: (_, _) => const SizedBox(height: 8),
        itemBuilder: (context, i) {
          final order = orders[i];
          final next = OrderStatus.next(order.status);
          final busy = _busyOrder == order.id;
          return Column(
            children: [
              OrderCard(order: order, showCustomer: true),
              if (next != null || OrderStatus.canCancel(order.status))
                Row(
                  children: [
                    if (next != null)
                      Expanded(
                        child: FilledButton(
                          onPressed: busy ? null : () => _advance(order),
                          child: Text(OrderStatus.actionLabel(order.status)),
                        ),
                      ),
                    if (next != null && OrderStatus.canCancel(order.status))
                      const SizedBox(width: 8),
                    if (OrderStatus.canCancel(order.status))
                      Expanded(
                        child: OutlinedButton(
                          onPressed: busy ? null : () => _confirmCancel(order),
                          child: const Text('Cancel'),
                        ),
                      ),
                  ],
                ),
            ],
          );
        },
      ),
    );
  }
}
