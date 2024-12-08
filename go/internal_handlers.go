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
	if len(chairs) == 0 {
		slog.Info("No active chairs")
		return
	}

	chairIDs := make([]string, 0, len(chairs))
	for _, chair := range chairs {
		chairIDs = append(chairIDs, chair.ID)
	}

	var nullRides []*Ride
	if err := database().SelectContext(ctx, &nullRides, "SELECT * FROM rides WHERE chair_id IS NULL"); err != nil {
		slog.Error("Failed to fetch rides", err)
		return
	}
	if len(nullRides) == 0 {
		slog.Info("No rides to match in chairs")
		return
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
	if len(rides) == 0 {
		slog.Info("No rides to match in chairs")
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

	for _, nullRide := range nullRides {
		for _, chair := range chairs {
			ridesInChair := 0
			for _, ride := range rides {
				if ride.ChairID.String == chair.ID {
					ridesInChair++
				}
			}
			if ridesInChair < 6 {
				slog.Info("Matched ride", nullRide.ID, chair.ID)
				if _, err := database().ExecContext(ctx, "UPDATE rides SET chair_id = ? WHERE id = ?", chair.ID, nullRide.ID); err != nil {
					slog.Error("Failed to update ride", err)
					return
				}
				break
			}
		}
	}
}
