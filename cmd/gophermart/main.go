package main

import (
	"context"
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
	"gophermart/internal/worker"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	orderProcessor := worker.NewOrderProcessor(orderService, accrualClient)
	go orderProcessor.Run(ctx)

	go func() {
		log.Printf("starting server on %s", cfg.RunAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}
