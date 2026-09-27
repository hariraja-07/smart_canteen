import 'package:flutter/material.dart';

import '../../core/async_view.dart';
import '../../core/models.dart';
import '../../core/session.dart';

/// The orders this user is allowed to see.
///
/// There is no filter argument, and that is deliberate: the server decides the
/// scope from the token, so a student cannot ask for someone else's orders and
/// there is no userId on this screen to get wrong.
class OrdersPage extends StatelessWidget {
  const OrdersPage({super.key});

  @override
  Widget build(BuildContext context) {
    final session = SessionScope.of(context);
    return AsyncView<List<Order>>(
      load: session.api.fetchOrders,
      refreshable: true,
      isEmpty: (orders) => orders.isEmpty,
      emptyMessage: session.canManageKitchen
          ? 'No orders in the queue'
          : 'You have not ordered yet',
      builder: (context, orders) => ListView.separated(
        padding: const EdgeInsets.all(12),
        itemCount: orders.length,
        separatorBuilder: (_, _) => const SizedBox(height: 8),
        itemBuilder: (context, i) => OrderCard(
          order: orders[i],
          // The customer's name is only meaningful to whoever can see the whole
          // queue; a student looking at their own order has nothing to read.
          showCustomer: session.canManageKitchen,
        ),
      ),
    );
  }
}

class OrderCard extends StatelessWidget {
  final Order order;
  final bool showCustomer;

  const OrderCard({super.key, required this.order, this.showCustomer = false});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Card(
      margin: EdgeInsets.zero,
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Text(
                  'Order #${order.id}',
                  style: theme.textTheme.titleMedium?.copyWith(
                    fontWeight: FontWeight.bold,
                  ),
                ),
                const Spacer(),
                OrderStatusChip(status: order.status),
              ],
            ),
            if (showCustomer) ...[
              const SizedBox(height: 4),
              Text(order.customer, style: theme.textTheme.bodyMedium),
            ],
            const SizedBox(height: 8),
            for (final item in order.items)
              Padding(
                padding: const EdgeInsets.only(bottom: 2),
                child: Text(
                  '${item.qty} x ${item.name}  ${item.lineTotal}',
                  style: theme.textTheme.bodySmall,
                ),
              ),
            const Divider(height: 16),
            Row(
              children: [
                Text('Total', style: theme.textTheme.bodyMedium),
                const Spacer(),
                Text('${order.total} coins', style: theme.textTheme.titleSmall),
              ],
            ),
            const SizedBox(height: 2),
            Text(shortDate(order.createdAt), style: theme.textTheme.bodySmall),
          ],
        ),
      ),
    );
  }
}

class OrderStatusChip extends StatelessWidget {
  final String status;

  const OrderStatusChip({super.key, required this.status});

  @override
  Widget build(BuildContext context) {
    // Colours match the order's meaning, so a queue can be read at a glance.
    final (Color bg, Color fg) = switch (status) {
      OrderStatus.pending => (Colors.amber.shade100, Colors.amber.shade900),
      OrderStatus.preparing => (Colors.blue.shade100, Colors.blue.shade900),
      OrderStatus.ready => (Colors.green.shade100, Colors.green.shade900),
      OrderStatus.completed => (Colors.grey.shade200, Colors.grey.shade800),
      OrderStatus.cancelled => (Colors.red.shade100, Colors.red.shade900),
      _ => (Colors.grey.shade200, Colors.grey.shade800),
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        OrderStatus.label(status),
        style: TextStyle(color: fg, fontSize: 12, fontWeight: FontWeight.w600),
      ),
    );
  }
}
