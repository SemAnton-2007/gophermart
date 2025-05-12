package repository

import (
	"context"
	"database/sql"
	"errors"

	"gophermart/internal/model"
)

type OrderRepository struct {
	db *Database
}

func NewOrderRepository(db *Database) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) Create(ctx context.Context, order *model.Order) error {
	query := `INSERT INTO orders (number, status, user_id, uploaded_at) 
	          VALUES ($1, $2, $3, $4) 
	          ON CONFLICT (number) DO NOTHING`
	result, err := r.db.db.ExecContext(ctx, query, order.Number, order.Status, order.UserID, order.UploadedAt)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("order already exists")
	}

	return nil
}

func (r *OrderRepository) GetByNumber(ctx context.Context, number string) (*model.Order, error) {
	query := `SELECT number, status, accrual, uploaded_at, user_id FROM orders WHERE number = $1`
	order := &model.Order{}
	err := r.db.db.QueryRowContext(ctx, query, number).Scan(
		&order.Number,
		&order.Status,
		&order.Accrual,
		&order.UploadedAt,
		&order.UserID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return order, nil
}

func (r *OrderRepository) GetByUserID(ctx context.Context, userID int64) ([]model.Order, error) {
	query := `SELECT number, status, accrual, uploaded_at FROM orders 
	          WHERE user_id = $1 
	          ORDER BY uploaded_at DESC`
	rows, err := r.db.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []model.Order
	for rows.Next() {
		var order model.Order
		if err := rows.Scan(&order.Number, &order.Status, &order.Accrual, &order.UploadedAt); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return orders, nil
}

func (r *OrderRepository) Update(ctx context.Context, order *model.Order) error {
	query := `UPDATE orders SET status = $1, accrual = $2 WHERE number = $3`
	_, err := r.db.db.ExecContext(ctx, query, order.Status, order.Accrual, order.Number)
	return err
}

func (r *OrderRepository) GetUnprocessedOrders(ctx context.Context) ([]model.Order, error) {
	query := `SELECT number, status, user_id FROM orders 
	          WHERE status IN ('NEW', 'PROCESSING') 
	          ORDER BY uploaded_at`
	rows, err := r.db.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []model.Order
	for rows.Next() {
		var order model.Order
		if err := rows.Scan(&order.Number, &order.Status, &order.UserID); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return orders, nil
}

func (r *OrderRepository) DB() *sql.DB {
	return r.db.DB()
}
