package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gophermart/internal/model"
	"gophermart/internal/repository"
)

var (
	ErrOrderAlreadyUploadedByUser        = errors.New("order already uploaded by this user")
	ErrOrderAlreadyUploadedByAnotherUser = errors.New("order already uploaded by another user")
)

type OrderService struct {
	orderRepo *repository.OrderRepository
}

func NewOrderService(orderRepo *repository.OrderRepository) *OrderService {
	return &OrderService{orderRepo: orderRepo}
}

func (s *OrderService) CreateOrder(ctx context.Context, userID int64, number string) error {
	existingOrder, err := s.orderRepo.GetByNumber(ctx, number)
	if err != nil {
		return fmt.Errorf("failed to check order existence: %w", err)
	}

	if existingOrder != nil {
		if existingOrder.UserID == userID {
			return ErrOrderAlreadyUploadedByUser
		}
		return ErrOrderAlreadyUploadedByAnotherUser
	}

	newOrder := &model.Order{
		Number:     number,
		Status:     model.OrderStatusNew,
		UploadedAt: time.Now(),
		UserID:     userID,
	}

	if err := s.orderRepo.Create(ctx, newOrder); err != nil {
		return fmt.Errorf("failed to create order: %w", err)
	}

	return nil
}

func (s *OrderService) GetUserOrders(ctx context.Context, userID int64) ([]model.Order, error) {
	return s.orderRepo.GetByUserID(ctx, userID)
}

func (s *OrderService) GetUnprocessedOrders(ctx context.Context) ([]model.Order, error) {
	return s.orderRepo.GetUnprocessedOrders(ctx)
}

func (s *OrderService) UpdateOrder(ctx context.Context, order *model.Order) error {
	return s.orderRepo.Update(ctx, order)
}

func (s *OrderService) CalculateCurrentBalance(ctx context.Context, userID int64) (float64, error) {
	var balance float64
	query := `SELECT COALESCE(SUM(accrual), 0) - 
              COALESCE((SELECT SUM(sum) FROM withdrawals WHERE user_id = $1), 0)
              FROM orders WHERE user_id = $1 AND status = 'PROCESSED'`
	err := s.orderRepo.DB().QueryRowContext(ctx, query, userID).Scan(&balance)
	if err != nil {
		return 0, err
	}
	return balance, nil
}
