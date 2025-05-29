package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"gophermart/internal/config"
	"gophermart/internal/service"
	"gophermart/internal/service/accrual"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	cfg               *config.Config
	authService       *service.AuthService
	orderService      *service.OrderService
	withdrawalService *service.WithdrawalService
	accrualClient     *accrual.AccrualClient
	tokenAuth         *jwtauth.JWTAuth
}

func NewHandler(
	cfg *config.Config,
	authService *service.AuthService,
	orderService *service.OrderService,
	withdrawalService *service.WithdrawalService,
	accrualClient *accrual.AccrualClient,
	tokenAuth *jwtauth.JWTAuth,
) *Handler {
	return &Handler{
		cfg:               cfg,
		authService:       authService,
		orderService:      orderService,
		withdrawalService: withdrawalService,
		accrualClient:     accrualClient,
		tokenAuth:         tokenAuth,
	}
}

func (h *Handler) RegisterRoutes(r *chi.Mux) {
	r.Post("/api/user/register", h.Register)
	r.Post("/api/user/login", h.Login)

	r.Group(func(r chi.Router) {
		r.Use(jwtauth.Verifier(h.tokenAuth))
		r.Use(jwtauth.Authenticator)

		r.Post("/api/user/orders", h.UploadOrder)
		r.Get("/api/user/orders", h.GetOrders)
		r.Get("/api/user/balance", h.GetBalance)
		r.Post("/api/user/balance/withdraw", h.Withdraw)
		r.Get("/api/user/withdrawals", h.GetWithdrawals)
	})
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request format", http.StatusBadRequest)
		return
	}

	user, err := h.authService.Register(r.Context(), req.Login, req.Password)
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		if err.Error() == "user already exists" {
			http.Error(w, "login already taken", http.StatusConflict)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	_, tokenString, err := h.tokenAuth.Encode(map[string]interface{}{"user_id": user.ID})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Authorization", "Bearer "+tokenString)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request format", http.StatusBadRequest)
		return
	}

	user, err := h.authService.Login(r.Context(), req.Login, req.Password)
	if err != nil {
		if err.Error() == "invalid credentials" {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	_, tokenString, err := h.tokenAuth.Encode(map[string]interface{}{"user_id": user.ID})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Authorization", "Bearer "+tokenString)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) UploadOrder(w http.ResponseWriter, r *http.Request) {
	_, claims, _ := jwtauth.FromContext(r.Context())
	userID := int64(claims["user_id"].(float64))

	orderNumber, err := readOrderNumber(r)
	if err != nil {
		http.Error(w, "invalid order number format", http.StatusBadRequest)
		return
	}

	if !validateOrderNumber(orderNumber) {
		http.Error(w, "invalid order number", http.StatusUnprocessableEntity)
		return
	}

	err = h.orderService.CreateOrder(r.Context(), userID, orderNumber)
	if err != nil {
		if errors.Is(err, service.ErrOrderAlreadyUploadedByUser) {
			w.WriteHeader(http.StatusOK)
			return
		}
		if errors.Is(err, service.ErrOrderAlreadyUploadedByAnotherUser) {
			http.Error(w, "order already uploaded by another user", http.StatusConflict)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) GetOrders(w http.ResponseWriter, r *http.Request) {
	_, claims, _ := jwtauth.FromContext(r.Context())
	userID := int64(claims["user_id"].(float64))

	orders, err := h.orderService.GetUserOrders(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}

func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	_, claims, _ := jwtauth.FromContext(r.Context())
	userID := int64(claims["user_id"].(float64))

	currentBalance, err := h.orderService.CalculateCurrentBalance(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	withdrawn, err := h.withdrawalService.CalculateWithdrawn(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	balance := struct {
		Current   float64 `json:"current"`
		Withdrawn float64 `json:"withdrawn"`
	}{
		Current:   currentBalance,
		Withdrawn: withdrawn,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(balance)
}

func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	_, claims, _ := jwtauth.FromContext(r.Context())
	userID := int64(claims["user_id"].(float64))

	var req struct {
		Order string  `json:"order"`
		Sum   float64 `json:"sum"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request format", http.StatusBadRequest)
		return
	}

	if !validateOrderNumber(req.Order) {
		http.Error(w, "invalid order number", http.StatusUnprocessableEntity)
		return
	}

	err := h.withdrawalService.CreateWithdrawal(r.Context(), userID, req.Order, req.Sum)
	if err != nil {
		if err.Error() == "insufficient funds" {
			http.Error(w, "insufficient funds", http.StatusPaymentRequired)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) GetWithdrawals(w http.ResponseWriter, r *http.Request) {
	_, claims, _ := jwtauth.FromContext(r.Context())
	userID := int64(claims["user_id"].(float64))

	withdrawals, err := h.withdrawalService.GetUserWithdrawals(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if len(withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(withdrawals)
}

func readOrderNumber(r *http.Request) (string, error) {
	if r.Header.Get("Content-Type") == "application/json" {
		var number string
		if err := json.NewDecoder(r.Body).Decode(&number); err != nil {
			return "", err
		}
		return number, nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func validateOrderNumber(number string) bool {
	sum := 0
	alternate := false

	for i := len(number) - 1; i >= 0; i-- {
		digit, err := strconv.Atoi(string(number[i]))
		if err != nil {
			return false
		}

		if alternate {
			digit *= 2
			if digit > 9 {
				digit = (digit % 10) + 1
			}
		}

		sum += digit
		alternate = !alternate
	}

	return sum%10 == 0
}
