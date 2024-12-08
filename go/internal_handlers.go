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

	rideStatuses := []*RideStatus{}
	if len(rides) >= 0 {
		rideIDs := make([]string, 0, len(rides))
		for _, ride := range rides {
			rideIDs = append(rideIDs, ride.ID)
		}

		query, params, err = sqlx.In("SELECT * FROM ride_statuses WHERE ride_id IN (?) ORDER BY created_desc DESC", rideIDs)
		if err != nil {
			slog.Error("Failed to parse ride_statuses in query", err)
			return
		}
		if err := database().SelectContext(ctx, &rideStatuses, database().Rebind(query), params...); err != nil {
			slog.Error("Failed to fetch ride_statuses", err)
			return
		}
	}

	rideStatusesByRideID := make(map[string][]*RideStatus)
	for _, rideStatus := range rideStatuses {
		rideStatusesByRideID[rideStatus.RideID] = append(rideStatusesByRideID[rideStatus.RideID], rideStatus)
	}

	for _, nullRide := range nullRides {
		for _, chair := range chairs {
			ridesInChair := make([]*Ride, 0, 100)
			for _, ride := range rides {
				if ride.ChairID.String == chair.ID {
					ridesInChair = append(ridesInChair, ride)
				}
			}

			count := 0
			for _, ride := range ridesInChair {
				statuses, ok := rideStatusesByRideID[ride.ID]
				if !ok {
					continue
				}

				for _, status := range statuses {
					if status.ChairSentAt != nil {
						count++
					}
				}
			}

			if count < 6 {
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
