package main

import (
	"context"
	"gophermart/internal/model"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth"

	"gophermart/internal/config"
	"gophermart/internal/controller"
	"gophermart/internal/repository"
	"gophermart/internal/service"
	"gophermart/internal/service/accrual"
	"gophermart/migrations"
)

func main() {
	cfg := config.Load()

	db, err := repository.NewDatabase(cfg.DatabaseURI)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := migrations.ApplyMigrations(db.DB()); err != nil {
		log.Fatalf("failed to apply migrations: %v", err)
	}

	tokenAuth := jwtauth.New("HS256", []byte(cfg.JWTSecret), nil)

	userRepo := repository.NewUserRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	withdrawalRepo := repository.NewWithdrawalRepository(db)

	authService := service.NewAuthService(userRepo)
	orderService := service.NewOrderService(orderRepo)
	withdrawService := service.NewWithdrawalService(withdrawalRepo)
	accrualClient := accrual.NewAccrualClient(cfg.AccrualSystemAddress)

	handler := controller.NewHandler(
		cfg,
		authService,
		orderService,
		withdrawService,
		accrualClient,
		tokenAuth,
	)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	server := &http.Server{
		Addr:    cfg.RunAddress,
		Handler: r,
	}

	go processOrders(context.Background(), orderService, accrualClient)

	go func() {
		log.Printf("starting server on %s", cfg.RunAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}

func processOrders(ctx context.Context, orderService *service.OrderService, accrualClient *accrual.AccrualClient) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			orders, err := orderService.GetUnprocessedOrders(ctx)
			if err != nil {
				log.Printf("failed to get unprocessed orders: %v", err)
				continue
			}

			for _, order := range orders {
				resp, err := accrualClient.GetAccrual(ctx, order.Number)
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

				if err := orderService.UpdateOrder(ctx, &order); err != nil {
					log.Printf("failed to update order %s: %v", order.Number, err)
				}
			}
		}
	}
}
