package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jmoiron/sqlx"
	"github.com/oklog/ulid/v2"
)

type chairPostChairsRequest struct {
	Name               string `json:"name"`
	Model              string `json:"model"`
	ChairRegisterToken string `json:"chair_register_token"`
}

type chairPostChairsResponse struct {
	ID      string `json:"id"`
	OwnerID string `json:"owner_id"`
}

func chairPostChairs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	req := &chairPostChairsRequest{}
	if err := bindJSON(r, req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Model == "" || req.ChairRegisterToken == "" {
		writeError(w, http.StatusBadRequest, errors.New("some of required fields(name, model, chair_register_token) are empty"))
		return
	}

	owner := &Owner{}
	if err := database().GetContext(ctx, owner, "SELECT * FROM owners WHERE chair_register_token = ?", req.ChairRegisterToken); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, errors.New("invalid chair_register_token"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	chairID := ulid.Make().String()
	accessToken := secureRandomStr(32)

	_, err := database().ExecContext(
		ctx,
		"INSERT INTO chairs (id, owner_id, name, model, is_active, access_token) VALUES (?, ?, ?, ?, ?, ?)",
		chairID, owner.ID, req.Name, req.Model, false, accessToken,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Path:  "/",
		Name:  "chair_session",
		Value: accessToken,
	})

	writeJSON(w, http.StatusCreated, &chairPostChairsResponse{
		ID:      chairID,
		OwnerID: owner.ID,
	})
}

type postChairActivityRequest struct {
	IsActive bool `json:"is_active"`
}

func chairPostActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chair := ctx.Value("chair").(*Chair)

	req := &postChairActivityRequest{}
	if err := bindJSON(r, req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	_, err := database().ExecContext(ctx, "UPDATE chairs SET is_active = ? WHERE id = ?", req.IsActive, chair.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type chairPostCoordinateResponse struct {
	RecordedAt int64 `json:"recorded_at"`
}

func chairPostCoordinate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	req := &Coordinate{}
	if err := bindJSON(r, req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	chair := ctx.Value("chair").(*Chair)

	tx, err := database().Beginx()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()

	chairLocationID := ulid.Make().String()
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO chair_locations (id, chair_id, latitude, longitude) VALUES (?, ?, ?, ?)`,
		chairLocationID, chair.ID, req.Latitude, req.Longitude,
	); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	location := &ChairLocation{}
	if err := tx.GetContext(ctx, location, `SELECT * FROM chair_locations WHERE id = ?`, chairLocationID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// 距離の更新をするように
	var lastLocation ChairLocation
	err = tx.GetContext(ctx, &lastLocation, `
		SELECT * 
		FROM chair_locations 
		WHERE chair_id = ? 
		ORDER BY created_at DESC 
		LIMIT 1 OFFSET 1
	`, chair.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var distance int
	if err == nil {
		distance = abs(location.Latitude-lastLocation.Latitude) + abs(location.Longitude-lastLocation.Longitude)
		if _, err := tx.ExecContext(ctx, `
		UPDATE chairs
		SET total_distance = total_distance + ?, 
		    total_distance_updated_at = ?
		WHERE id = ?
	`, distance, location.CreatedAt, chair.ID); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	ride := &Ride{}
	if err := tx.GetContext(ctx, ride, `SELECT * FROM rides WHERE chair_id = ? ORDER BY updated_at DESC LIMIT 1`, chair.ID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	} else {
		status, err := getLatestRideStatus(ctx, tx, ride.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if status != "COMPLETED" && status != "CANCELED" {
			if req.Latitude == ride.PickupLatitude && req.Longitude == ride.PickupLongitude && status == "ENROUTE" {
				rideStatus := RideStatus{
					ID:     ulid.Make().String(),
					RideID: ride.ID,
					Status: "PICKUP",
				}
				if _, err := tx.ExecContext(
					ctx,
					`INSERT INTO ride_statuses (id, ride_id, status) VALUES (?, ?, ?)`,
					rideStatus.ID, rideStatus.RideID, rideStatus.Status,
				); err != nil {
					writeError(w, http.StatusInternalServerError, err)
					return
				}
				ch, ok := updateRideStatusCh[chair.ID]
				if !ok {
					ch = make(chan *RideRideStatus, 1)
					updateRideStatusCh[chair.ID] = ch
				}
				ch <- &RideRideStatus{r: ride, s: &rideStatus}
			}

			if req.Latitude == ride.DestinationLatitude && req.Longitude == ride.DestinationLongitude && status == "CARRYING" {
				rideStatus := RideStatus{
					ID:     ulid.Make().String(),
					RideID: ride.ID,
					Status: "ARRIVED",
				}
				if _, err := tx.ExecContext(
					ctx,
					`INSERT INTO ride_statuses (id, ride_id, status) VALUES (?, ?, ?)`,
					rideStatus.ID, rideStatus.RideID, rideStatus.Status,
				); err != nil {
					writeError(w, http.StatusInternalServerError, err)
					return
				}
				ch, ok := updateRideStatusCh[chair.ID]
				if !ok {
					ch = make(chan *RideRideStatus, 1)
					updateRideStatusCh[chair.ID] = ch
				}
				ch <- &RideRideStatus{r: ride, s: &rideStatus}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, &chairPostCoordinateResponse{
		RecordedAt: location.CreatedAt.UnixMilli(),
	})
}

type simpleUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type chairGetNotificationResponse struct {
	Data         *chairGetNotificationResponseData `json:"data"`
	RetryAfterMs int                               `json:"retry_after_ms"`
}

type chairGetNotificationResponseData struct {
	RideID                string     `json:"ride_id"`
	User                  simpleUser `json:"user"`
	PickupCoordinate      Coordinate `json:"pickup_coordinate"`
	DestinationCoordinate Coordinate `json:"destination_coordinate"`
	Status                string     `json:"status"`
}

type RideRideStatus struct {
	r *Ride
	s *RideStatus
}

var updateRideStatusCh = make(map[string]chan *RideRideStatus) // chairID -> chan *RideRideStatus

func chairGetNotification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chair := ctx.Value("chair").(*Chair)

	tx, err := database().Beginx()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()
	ride := &Ride{}
	user := &User{}

	if err := tx.GetContext(ctx, ride, `SELECT * FROM rides WHERE chair_id = ? ORDER BY updated_at DESC LIMIT 1`, chair.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			slog.Info("no ride found")
			writeJSON(w, http.StatusOK, &chairGetNotificationResponse{
				RetryAfterMs: 30,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	yetSentRideStatus := RideStatus{}
	status := ""
	if err := tx.GetContext(ctx, &yetSentRideStatus, `SELECT * FROM ride_statuses WHERE ride_id = ? AND chair_sent_at IS NULL ORDER BY created_at ASC LIMIT 1`, ride.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			status, err = getLatestRideStatus(ctx, tx, ride.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		} else {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	} else {
		status = yetSentRideStatus.Status
	}

	writeJSONForSSE(w, http.StatusOK, &chairGetNotificationResponse{
		Data: &chairGetNotificationResponseData{
			RideID: ride.ID,
			User: simpleUser{
				ID:   user.ID,
				Name: fmt.Sprintf("%s %s", user.Firstname, user.Lastname),
			},
			PickupCoordinate: Coordinate{
				Latitude:  ride.PickupLatitude,
				Longitude: ride.PickupLongitude,
			},
			DestinationCoordinate: Coordinate{
				Latitude:  ride.DestinationLatitude,
				Longitude: ride.DestinationLongitude,
			},
			Status: status,
		},
		RetryAfterMs: 30,
	})

	for {
		slog.Info("waiting for updateRideStatusCh", "chair.ID", chair.ID)
		select {
		case <-r.Context().Done():
			_ = tx.Commit()
			w.WriteHeader(http.StatusOK)
			return
		case rrs := <-updateRideStatusCh[chair.ID]:
			slog.Info("chairGetNotification", "rrs", rrs)
			err := tx.GetContext(ctx, user, "SELECT * FROM users WHERE id = ?", rrs.r.UserID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			chairSendNotification(w, r, tx, user, rrs.r, rrs.s)
		}
	}
}

func chairSendNotification(w http.ResponseWriter, r *http.Request, tx *sqlx.Tx, user *User, ride *Ride, status *RideStatus) {
	ctx := r.Context()

	// if status.ID != "" {
	_, err := tx.ExecContext(ctx, `UPDATE ride_statuses SET chair_sent_at = CURRENT_TIMESTAMP(6) WHERE id = ?`, status.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// }

	writeJSONForSSE(w, http.StatusOK, &chairGetNotificationResponse{
		Data: &chairGetNotificationResponseData{
			RideID: ride.ID,
			User: simpleUser{
				ID:   user.ID,
				Name: fmt.Sprintf("%s %s", user.Firstname, user.Lastname),
			},
			PickupCoordinate: Coordinate{
				Latitude:  ride.PickupLatitude,
				Longitude: ride.PickupLongitude,
			},
			DestinationCoordinate: Coordinate{
				Latitude:  ride.DestinationLatitude,
				Longitude: ride.DestinationLongitude,
			},
			Status: status.Status,
		},
		RetryAfterMs: 30,
	})
}

type postChairRidesRideIDStatusRequest struct {
	Status string `json:"status"`
}

func chairPostRideStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rideID := r.PathValue("ride_id")

	chair := ctx.Value("chair").(*Chair)

	req := &postChairRidesRideIDStatusRequest{}
	if err := bindJSON(r, req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	tx, err := database().Beginx()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()

	ride := &Ride{}
	if err := tx.GetContext(ctx, ride, "SELECT * FROM rides WHERE id = ? FOR UPDATE", rideID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, errors.New("ride not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if ride.ChairID.String != chair.ID {
		writeError(w, http.StatusBadRequest, errors.New("not assigned to this ride"))
		return
	}

	switch req.Status {
	// Acknowledge the ride
	case "ENROUTE":
		rideStatus := RideStatus{
			ID:     ulid.Make().String(),
			RideID: ride.ID,
			Status: "ENROUTE",
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO ride_statuses (id, ride_id, status) VALUES (?, ?, ?)`,
			rideStatus.ID, rideStatus.RideID, rideStatus.Status,
		); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		ch, ok := updateRideStatusCh[chair.ID]
		if !ok {
			ch = make(chan *RideRideStatus, 1)
			updateRideStatusCh[chair.ID] = ch
		}
		ch <- &RideRideStatus{r: ride, s: &rideStatus}
	// After Picking up user
	case "CARRYING":
		status, err := getLatestRideStatus(ctx, tx, ride.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if status != "PICKUP" {
			writeError(w, http.StatusBadRequest, errors.New("chair has not arrived yet"))
			return
		}
		rideStatus := RideStatus{
			ID:     ulid.Make().String(),
			RideID: ride.ID,
			Status: "CARRYING",
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO ride_statuses (id, ride_id, status) VALUES (?, ?, ?)`,
			rideStatus.ID, rideStatus.RideID, rideStatus.Status,
		); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		ch, ok := updateRideStatusCh[chair.ID]
		if !ok {
			ch = make(chan *RideRideStatus, 1)
			updateRideStatusCh[chair.ID] = ch
		}
		ch <- &RideRideStatus{r: ride, s: &rideStatus}
	default:
		writeError(w, http.StatusBadRequest, errors.New("invalid status"))
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
