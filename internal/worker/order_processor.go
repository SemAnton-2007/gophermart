package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	"gophermart/internal/model"
	"gophermart/internal/service"
	"gophermart/internal/service/accrual"
)

type OrderProcessor struct {
	orderService   *service.OrderService
	accrualClient  *accrual.AccrualClient
	maxWorkers     int64
	processTimeout time.Duration
}

func NewOrderProcessor(
	orderService *service.OrderService,
	accrualClient *accrual.AccrualClient,
	opts ...Option,
) *OrderProcessor {
	p := &OrderProcessor{
		orderService:   orderService,
		accrualClient:  accrualClient,
		maxWorkers:     5,
		processTimeout: 30 * time.Second,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

type Option func(*OrderProcessor)

func WithMaxWorkers(n int64) Option {
	return func(p *OrderProcessor) {
		p.maxWorkers = n
	}
}

func WithProcessTimeout(d time.Duration) Option {
	return func(p *OrderProcessor) {
		p.processTimeout = d
	}
}

func (p *OrderProcessor) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	sem := semaphore.NewWeighted(p.maxWorkers)
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-ticker.C:
			p.processBatch(ctx, sem, &wg)
		}
	}
}

func (p *OrderProcessor) processBatch(ctx context.Context, sem *semaphore.Weighted, wg *sync.WaitGroup) {
	orders, err := p.orderService.GetUnprocessedOrders(ctx)
	if err != nil {
		log.Printf("failed to get unprocessed orders: %v", err)
		return
	}

	for _, order := range orders {
		if err := sem.Acquire(ctx, 1); err != nil {
			continue
		}

		wg.Add(1)
		go func(o model.Order) {
			defer sem.Release(1)
			defer wg.Done()
			p.processOrder(ctx, o)
		}(order)
	}
}

func (p *OrderProcessor) processOrder(ctx context.Context, order model.Order) {
	ctx, cancel := context.WithTimeout(ctx, p.processTimeout)
	defer cancel()

	resp, err := p.accrualClient.GetAccrual(ctx, order.Number)
	if err != nil {
		log.Printf("failed to get accrual for order %s: %v", order.Number, err)
		return
	}

	if resp == nil {
		return
	}

	order.Status = resp.Status
	if resp.Status == model.OrderStatusProcessed {
		order.Accrual = resp.Accrual
	}

	if err := p.orderService.UpdateOrder(ctx, &order); err != nil {
		log.Printf("failed to update order %s: %v", order.Number, err)
	}
}
