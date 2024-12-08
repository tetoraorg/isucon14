package main

import (
	"context"
	"log/slog"

	"github.com/jmoiron/sqlx"
)

// このAPIをインスタンス内から一定間隔で叩かせることで、椅子とライドをマッチングさせる
func internalGetMatching(ctx context.Context) {
	var chairs []*Chair
	if err := database().SelectContext(ctx, &chairs, "SELECT * FROM chairs WHERE is_active = TRUE"); err != nil {
		slog.Error("Failed to fetch chairs", err)
		return
	}

	chairIDs := make([]string, 0, len(chairs))
	for _, chair := range chairs {
		chairIDs = append(chairIDs, chair.ID)
	}

	var rides []*Ride
	query, params, err := sqlx.In("SELECT * FROM rides WHERE chair_id IN (?)", chairIDs)
	if err != nil {
		slog.Error("Failed to parse rides in query", err)
		return
	}
	if err := database().SelectContext(ctx, &rides, database().Rebind(query), params...); err != nil {
		slog.Error("Failed to fetch rides", err)
		return
	}

	rideIDs := make([]string, 0, len(rides))
	for _, ride := range rides {
		rideIDs = append(rideIDs, ride.ID)
	}

	var rideStatuses []*RideStatus
	query, params, err = sqlx.In("SELECT * FROM ride_statuses WHERE ride_id IN (?)", rideIDs)
	if err != nil {
		slog.Error("Failed to parse ride_statuses in query", err)
		return
	}
	if err := database().SelectContext(ctx, &rideStatuses, database().Rebind(query), params...); err != nil {
		slog.Error("Failed to fetch ride_statuses", err)
		return
	}

	var matchedRide *Ride
	for _, ride := range rides {
		if len(rideStatuses) == 0 {
			matchedRide = ride
			break
		}

		var count int
		for _, rideStatus := range rideStatuses {
			if rideStatus.RideID == ride.ID {
				count++
			}
		}

		if count < 6 {
			matchedRide = ride
			break
		}
	}

	if _, err := database().ExecContext(ctx, "UPDATE rides SET chair_id = ? WHERE id = ?", matchedRide.ChairID, matchedRide.ID); err != nil {
		slog.Error("Failed to update ride", err)
		return
	}

}
