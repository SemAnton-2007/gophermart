package worker

import (
	"context"
	"log"
	"time"

	"gophermart/internal/model"
	"gophermart/internal/service"
	"gophermart/internal/service/accrual"
)

type OrderProcessor struct {
	orderService  *service.OrderService
	accrualClient *accrual.AccrualClient
}

func NewOrderProcessor(orderService *service.OrderService, accrualClient *accrual.AccrualClient) *OrderProcessor {
	return &OrderProcessor{
		orderService:  orderService,
		accrualClient: accrualClient,
	}
}

func (p *OrderProcessor) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.processOrders(ctx)
		}
	}
}

func (p *OrderProcessor) processOrders(ctx context.Context) {
	orders, err := p.orderService.GetUnprocessedOrders(ctx)
	if err != nil {
		log.Printf("failed to get unprocessed orders: %v", err)
		return
	}

	for _, order := range orders {
		resp, err := p.accrualClient.GetAccrual(ctx, order.Number)
		if err != nil {
			log.Printf("failed to get accrual for order %s: %v", order.Number, err)
			continue
		}

		if resp == nil {
			continue
		}

		order.Status = resp.Status
		if resp.Status == model.OrderStatusProcessed {
			order.Accrual = resp.Accrual
		}

		if err := p.orderService.UpdateOrder(ctx, &order); err != nil {
			log.Printf("failed to update order %s: %v", order.Number, err)
		}
	}
}
