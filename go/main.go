package main

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/kaz/pprotein/integration"
	"github.com/motoki317/sc"
)

var userByIDCache, _ = sc.New(func(ctx context.Context, id string) (*User, error) {
	var user User
	query := "SELECT * FROM users WHERE id = ?"
	err := database().GetContext(ctx, &user, query, id)
	return &user, err
}, 90*time.Second, 90*time.Second)

var userByTokenCache, _ = sc.New(func(ctx context.Context, token string) (*User, error) {
	var user User
	query := "SELECT * FROM users WHERE access_token = ?"
	err := database().GetContext(ctx, &user, query, token)
	return &user, err
}, 90*time.Second, 90*time.Second)

var userByInviteCache, _ = sc.New(func(ctx context.Context, invite string) (*User, error) {
	var user User
	query := "SELECT * FROM users WHERE invitation_code = ?"
	err := database().GetContext(ctx, &user, query, invite)
	return &user, err
}, 90*time.Second, 90*time.Second)

var ownerByIDCache, _ = sc.New(func(ctx context.Context, id string) (*Owner, error) {
	var owner Owner
	query := "SELECT * FROM owners WHERE id = ?"
	err := database().GetContext(ctx, &owner, query, id)
	return &owner, err
}, 90*time.Second, 90*time.Second)

var ownerByTokenCache, _ = sc.New(func(ctx context.Context, token string) (*Owner, error) {
	var owner Owner
	query := "SELECT * FROM owners WHERE access_token = ?"
	err := database().GetContext(ctx, &owner, query, token)
	return &owner, err
}, 90*time.Second, 90*time.Second)

var ownerByRegisterCache, _ = sc.New(func(ctx context.Context, register string) (*Owner, error) {
	var owner Owner
	query := "SELECT * FROM owners WHERE chair_register_token = ?"
	err := database().GetContext(ctx, &owner, query, register)
	return &owner, err
}, 90*time.Second, 90*time.Second)

var settingCache, _ = sc.New(func(ctx context.Context, name string) (string, error) {
	var setting string
	query := "SELECT value FROM settings WHERE name = ?"
	err := database().GetContext(ctx, &setting, query, name)
	return setting, err
}, 90*time.Second, 90*time.Second)

var paymentTokenCache, _ = sc.New(func(ctx context.Context, userID string) (*PaymentToken, error) {
	var paymentToken PaymentToken
	query := "SELECT * FROM payment_tokens WHERE user_id = ?"
	err := database().GetContext(ctx, &paymentToken, query, userID)
	return &paymentToken, err
}, 90*time.Second, 90*time.Second)

// var latestRideStatusCache, _ = sc.New(func(ctx context.Context, rideID string) (string, error) {
// 	status := ""
// 	if err := ridesDatabase().GetContext(ctx, &status, `SELECT status FROM ride_statuses WHERE ride_id = ? ORDER BY created_at DESC LIMIT 1`, rideID); err != nil {
// 		return "", err
// 	}

//		return status, nil
//	}, 90*time.Second, 90*time.Second)
func init() {
	slog.SetDefault(
		slog.New(
			slog.NewTextHandler(
				os.Stdout,
				&slog.HandlerOptions{Level: slog.LevelError},
			),
		),
	)
}

var chairLocationsCache = sync.Map{}

func main() {
	mux := setup()

	go func() {
		interval := 500 // milli seconds
		if vStr, exists := os.LookupEnv("ISUCON_MATCHING_INTERVAL"); exists {
			if val, err := strconv.Atoi(vStr); err == nil {
				interval = val
			}
		}
		if interval == 0 {
			return
		}

		ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			internalGetMatching(context.Background())
		}
	}()

	slog.Info("Listening on :8080")
	http.ListenAndServe(":8080", mux)
}

func setup() http.Handler {

	initDatabase()

	// 再起動試験対策
	for {
		err := database().Ping()
		err2 := ridesDatabase().Ping()
		if err == nil && err2 == nil {
			break
		}

		if err != nil {
			slog.Error("DB not ready", err)
		}

		if err2 != nil {
			slog.Error("Rides DB not ready", err2)
		}
		time.Sleep(time.Second * 2)
	}
	slog.Info("DB ready")

	mux := chi.NewRouter()
	if os.Getenv("PROD") != "true" {
		mux.Use(middleware.Logger)
		mux.Use(middleware.Recoverer)
	}
	mux.HandleFunc("POST /api/initialize", postInitialize)

	// app handlers
	{
		mux.HandleFunc("POST /api/app/users", appPostUsers)

		authedMux := mux.With(appAuthMiddleware)
		authedMux.HandleFunc("POST /api/app/payment-methods", appPostPaymentMethods)
		authedMux.HandleFunc("GET /api/app/rides", appGetRides)
		authedMux.HandleFunc("POST /api/app/rides", appPostRides)
		authedMux.HandleFunc("POST /api/app/rides/estimated-fare", appPostRidesEstimatedFare)
		authedMux.HandleFunc("POST /api/app/rides/{ride_id}/evaluation", appPostRideEvaluatation)
		authedMux.HandleFunc("GET /api/app/notification", appGetNotification)
		authedMux.HandleFunc("GET /api/app/nearby-chairs", appGetNearbyChairs)
	}

	// owner handlers
	{
		mux.HandleFunc("POST /api/owner/owners", ownerPostOwners)

		authedMux := mux.With(ownerAuthMiddleware)
		authedMux.HandleFunc("GET /api/owner/sales", ownerGetSales)
		authedMux.HandleFunc("GET /api/owner/chairs", ownerGetChairs)
	}

	// chair handlers
	{
		mux.HandleFunc("POST /api/chair/chairs", chairPostChairs)

		authedMux := mux.With(chairAuthMiddleware)
		authedMux.HandleFunc("POST /api/chair/activity", chairPostActivity)
		authedMux.HandleFunc("POST /api/chair/coordinate", chairPostCoordinate)
		authedMux.HandleFunc("GET /api/chair/notification", chairGetNotification)
		authedMux.HandleFunc("POST /api/chair/rides/{ride_id}/status", chairPostRideStatus)
	}

	// internal handlers
	// {
	// 	mux.HandleFunc("GET /api/internal/matching", internalGetMatching)
	// }

	// pproteinのエンドポイント設定
	if os.Getenv("PROD") != "true" {
		// ポートを分離したいときなど
		pproteinHandler := integration.NewDebugHandler()
		go http.ListenAndServe(":9000", pproteinHandler)
	}

	return mux
}

type postInitializeRequest struct {
	PaymentServer string `json:"payment_server"`
}

type postInitializeResponse struct {
	Language string `json:"language"`
}

func postInitialize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	req := &postInitializeRequest{}
	if err := bindJSON(r, req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if out, err := exec.Command("../sql/init.sh").CombinedOutput(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to initialize: %s: %w", string(out), err))
		return
	}

	if _, err := database().ExecContext(ctx, "UPDATE settings SET value = ? WHERE name = 'payment_gateway_url'", req.PaymentServer); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	settingCache.Purge()
	paymentTokenCache.Purge()

	userByIDCache.Purge()
	userByTokenCache.Purge()
	userByInviteCache.Purge()
	chairAccessTokenCache.Purge()

	ownerByIDCache.Purge()
	ownerByTokenCache.Purge()
	ownerByRegisterCache.Purge()

	// pproteinにcollect requestを飛ばす
	if os.Getenv("PROD") != "true" {
		go func() {
			if _, err := http.Get("https://pprotein-cqdme5gvfcg7gwew.australiaeast-01.azurewebsites.net/api/group/collect"); err != nil {
				writeError(w, http.StatusInternalServerError, err)
			}
		}()
	}

	writeJSON(w, http.StatusOK, postInitializeResponse{Language: "go"})
}

type Coordinate struct {
	Latitude  int `json:"latitude"`
	Longitude int `json:"longitude"`
}

func bindJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func writeJSON(w http.ResponseWriter, statusCode int, v interface{}) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	buf, err := json.Marshal(v)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(statusCode)
	w.Write(buf)
}

func writeError(w http.ResponseWriter, statusCode int, err error) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.WriteHeader(statusCode)
	buf, marshalError := json.Marshal(map[string]string{"message": err.Error()})
	if marshalError != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"marshaling error failed"}`))
		return
	}
	w.Write(buf)

	slog.Error("error response wrote", err)
}

func secureRandomStr(b int) string {
	k := make([]byte, b)
	if _, err := crand.Read(k); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", k)
}
