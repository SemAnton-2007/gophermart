package repository

import (
	"context"
	"database/sql"

	"gophermart/internal/model"
)

type WithdrawalRepository struct {
	db *Database
}

func NewWithdrawalRepository(db *Database) *WithdrawalRepository {
	return &WithdrawalRepository{db: db}
}

func (r *WithdrawalRepository) Create(ctx context.Context, withdrawal *model.Withdrawal) error {
	tx, err := r.db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentBalance float64
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(accrual), 0) - 
	                             COALESCE((SELECT SUM(sum) FROM withdrawals WHERE user_id = $1), 0) 
	                             FROM orders WHERE user_id = $1`, withdrawal.UserID).Scan(&currentBalance)
	if err != nil {
		return err
	}

	if currentBalance < withdrawal.Sum {
		return sql.ErrNoRows
	}

	query := `INSERT INTO withdrawals (order_number, sum, processed_at, user_id) 
	          VALUES ($1, $2, $3, $4)`
	_, err = tx.ExecContext(ctx, query, withdrawal.Order, withdrawal.Sum, withdrawal.ProcessedAt, withdrawal.UserID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *WithdrawalRepository) GetByUserID(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	query := `SELECT order_number, sum, processed_at FROM withdrawals 
	          WHERE user_id = $1 
	          ORDER BY processed_at DESC`
	rows, err := r.db.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var withdrawals []model.Withdrawal
	for rows.Next() {
		var w model.Withdrawal
		if err := rows.Scan(&w.Order, &w.Sum, &w.ProcessedAt); err != nil {
			return nil, err
		}
		withdrawals = append(withdrawals, w)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return withdrawals, nil
}

func (r *WithdrawalRepository) DB() *sql.DB {
	return r.db.DB()
}
