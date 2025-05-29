package repository

import (
	"context"
	"database/sql"
	"errors"

	"gophermart/internal/model"
)

type UserRepository struct {
	db *Database
}

func NewUserRepository(db *Database) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	query := `INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id`
	err := r.db.db.QueryRowContext(ctx, query, user.Login, user.PasswordHash).Scan(&user.ID)
	if err != nil {
		return err
	}
	return nil
}

func (r *UserRepository) GetByLogin(ctx context.Context, login string) (*model.User, error) {
	query := `SELECT id, login, password_hash FROM users WHERE login = $1`
	user := &model.User{}
	err := r.db.db.QueryRowContext(ctx, query, login).Scan(&user.ID, &user.Login, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	query := `SELECT id, login, password_hash FROM users WHERE id = $1`
	user := &model.User{}
	err := r.db.db.QueryRowContext(ctx, query, id).Scan(&user.ID, &user.Login, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}
