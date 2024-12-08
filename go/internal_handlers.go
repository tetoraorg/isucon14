package main

import (
	"context"
	"log/slog"

	"github.com/jmoiron/sqlx"
)

// このAPIをインスタンス内から一定間隔で叩かせることで、椅子とライドをマッチングさせる
func internalGetMatching(ctx context.Context) {
	tx, err := database().BeginTxx(ctx, nil)
	if err != nil {
		slog.Error("Failed to begin transaction", err)
		return
	}
	defer tx.Rollback()

	var chairs []*Chair
	if err := tx.SelectContext(ctx, &chairs, "SELECT * FROM chairs WHERE is_active = TRUE"); err != nil {
		slog.Error("Failed to fetch chairs", err)
		return
	}
	if len(chairs) == 0 {
		return
	}

	chairIDs := make([]string, 0, len(chairs))
	for _, chair := range chairs {
		chairIDs = append(chairIDs, chair.ID)
	}

	var nullRides []*Ride
	if err := tx.SelectContext(ctx, &nullRides, "SELECT * FROM rides WHERE chair_id IS NULL"); err != nil {
		slog.Error("Failed to fetch rides", err)
		return
	}
	if len(nullRides) == 0 {
		return
	}

	var rides []*Ride
	query, params, err := sqlx.In("SELECT * FROM rides WHERE chair_id IN (?)", chairIDs)
	if err != nil {
		slog.Error("Failed to parse rides in query", err)
		return
	}
	if err := tx.SelectContext(ctx, &rides, tx.Rebind(query), params...); err != nil {
		slog.Error("Failed to fetch rides", err)
		return
	}

	rideStatuses := []*RideStatus{}
	if len(rides) > 0 {
		rideIDs := make([]string, 0, len(rides))
		for _, ride := range rides {
			rideIDs = append(rideIDs, ride.ID)
		}

		query, params, err = sqlx.In("SELECT * FROM ride_statuses WHERE ride_id IN (?) ORDER BY created_at DESC", rideIDs)
		if err != nil {
			slog.Error("Failed to parse ride_statuses in query", err)
			return
		}
		if err := tx.SelectContext(ctx, &rideStatuses, tx.Rebind(query), params...); err != nil {
			slog.Error("Failed to fetch ride_statuses", err)
			return
		}
	}

	rideStatusesByRideID := make(map[string][]*RideStatus)
	for _, rideStatus := range rideStatuses {
		rideStatusesByRideID[rideStatus.RideID] = append(rideStatusesByRideID[rideStatus.RideID], rideStatus)
	}

	slog.Info("Matching rides", "len(chairs)", len(chairs), "len(nullRides)", len(nullRides), "len(rides)", len(rides))
	for _, nullRide := range nullRides {
		for _, chair := range chairs {
			ridesInChair := make([]*Ride, 0, 100)
			for _, ride := range rides {
				if ride.ChairID.String == chair.ID {
					ridesInChair = append(ridesInChair, ride)
				}
			}

			count := 0
			allReady := true
			for _, ride := range ridesInChair {
				statuses, ok := rideStatusesByRideID[ride.ID]
				if !ok {
					continue
				}

				if len(statuses) == 0 {
					continue
				}

				latestStatus := statuses[0]
				if latestStatus.Status != "COMPLETE" {
					allReady = false
					break
				}

				if latestStatus.ChairSentAt != nil {
					count++
				}
			}

			if count < 6 && allReady {
				slog.Info("Matched ride", nullRide.ID, chair.ID)
				if _, err := tx.ExecContext(ctx, "UPDATE rides SET chair_id = ? WHERE id = ?", chair.ID, nullRide.ID); err != nil {
					slog.Error("Failed to update ride", err)
					return
				}

				tx.Commit()
				return
			}
		}
	}
}
