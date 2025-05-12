package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gophermart/internal/model"
	"gophermart/internal/repository"
)

type WithdrawalService struct {
	withdrawalRepo *repository.WithdrawalRepository
}

func NewWithdrawalService(withdrawalRepo *repository.WithdrawalRepository) *WithdrawalService {
	return &WithdrawalService{withdrawalRepo: withdrawalRepo}
}

func (s *WithdrawalService) CreateWithdrawal(ctx context.Context, userID int64, order string, sum float64) error {
	withdrawal := &model.Withdrawal{
		Order:       order,
		Sum:         sum,
		ProcessedAt: time.Now(),
		UserID:      userID,
	}

	err := s.withdrawalRepo.Create(ctx, withdrawal)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("insufficient funds")
	}
	if err != nil {
		return err
	}

	return nil
}

func (s *WithdrawalService) GetUserWithdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	return s.withdrawalRepo.GetByUserID(ctx, userID)
}

func (s *WithdrawalService) CalculateWithdrawn(ctx context.Context, userID int64) (float64, error) {
	var withdrawn float64
	query := `SELECT COALESCE(SUM(sum), 0) FROM withdrawals WHERE user_id = $1`
	err := s.withdrawalRepo.DB().QueryRowContext(ctx, query, userID).Scan(&withdrawn)
	if err != nil {
		return 0, err
	}
	return withdrawn, nil
}
