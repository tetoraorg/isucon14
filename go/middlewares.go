package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/motoki317/sc"
)

func appAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		c, err := r.Cookie("app_session")
		if errors.Is(err, http.ErrNoCookie) || c.Value == "" {
			writeError(w, http.StatusUnauthorized, errors.New("app_session cookie is required"))
			return
		}
		accessToken := c.Value
		// user := &User{}
		// err = database().GetContext(ctx, user, "SELECT * FROM users WHERE access_token = ?", accessToken)
		// if err != nil {
		// 	if errors.Is(err, sql.ErrNoRows) {
		// 		writeError(w, http.StatusUnauthorized, errors.New("invalid access token"))
		// 		return
		// 	}
		// 	writeError(w, http.StatusInternalServerError, err)
		// 	return
		// }

		user, err := userByTokenCache.Get(ctx, accessToken)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, errors.New("invalid access token"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		ctx = context.WithValue(ctx, "user", user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ownerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		c, err := r.Cookie("owner_session")
		if errors.Is(err, http.ErrNoCookie) || c.Value == "" {
			writeError(w, http.StatusUnauthorized, errors.New("owner_session cookie is required"))
			return
		}
		accessToken := c.Value
		// owner := &Owner{}
		// if err := database().GetContext(ctx, owner, "SELECT * FROM owners WHERE access_token = ?", accessToken); err != nil {
		// 	if errors.Is(err, sql.ErrNoRows) {
		// 		writeError(w, http.StatusUnauthorized, errors.New("invalid access token"))
		// 		return
		// 	}
		// 	writeError(w, http.StatusInternalServerError, err)
		// 	return
		// }

		owner, err := ownerByTokenCache.Get(ctx, accessToken)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, errors.New("invalid access token"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		ctx = context.WithValue(ctx, "owner", owner)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func chairAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		c, err := r.Cookie("chair_session")
		if errors.Is(err, http.ErrNoCookie) || c.Value == "" {
			writeError(w, http.StatusUnauthorized, errors.New("chair_session cookie is required"))
			return
		}
		accessToken := c.Value
		chairOnlyNoChange, err := chairAccessTokenCache.Get(ctx, accessToken)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, errors.New("invalid access token"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		ctx = context.WithValue(ctx, "chairOnlyNoChange", chairOnlyNoChange)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

var chairAccessTokenCache, _ = sc.New(func(ctx context.Context, key string) (*ChairOnlyNoChange, error) {
	chair := Chair{}
	err := database().GetContext(ctx, &chair, "SELECT * FROM chairs WHERE access_token = ?", key)
	if err != nil {
		return &ChairOnlyNoChange{}, err
	}
	chairOnlyNoChange := &ChairOnlyNoChange{
		ID:          chair.ID,
		OwnerID:     chair.OwnerID,
		Name:        chair.Name,
		Model:       chair.Model,
		AccessToken: chair.AccessToken,
		CreatedAt:   chair.CreatedAt,
		UpdatedAt:   chair.UpdatedAt,
	}

	return chairOnlyNoChange, nil
}, 1*time.Minute, 5*time.Minute)
