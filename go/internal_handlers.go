package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
)

// このAPIをインスタンス内から一定間隔で叩かせることで、椅子とライドをマッチングさせる
func internalGetMatching(ctx context.Context) {
	const maxBatchSize = 10

	var rides []Ride
	if err := database().SelectContext(ctx, &rides, `SELECT * FROM rides WHERE chair_id IS NULL ORDER BY created_at LIMIT ?`, maxBatchSize); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return
		}
		slog.Error("Failed to fetch rides", err)
		return
	}

	if len(rides) == 0 {
		return
	}

	// 利用可能な椅子を一括取得
	var chairs []Chair
	if err := database().SelectContext(ctx, &chairs, `
		SELECT * FROM chairs
		WHERE is_active = TRUE AND id NOT IN (
			SELECT chair_id
			FROM (
				SELECT chair_id, COUNT(chair_sent_at) = 6 AS completed
				FROM ride_statuses
				WHERE ride_id IN (SELECT id FROM rides WHERE chair_id IS NOT NULL)
				GROUP BY chair_id
			) AS completed_chairs
			WHERE completed = FALSE
		)
		ORDER BY RAND()
		LIMIT ?
	`, len(rides)); err != nil { // 必要な数だけ取得
		if errors.Is(err, sql.ErrNoRows) {
			slog.Warn("No available chairs found")
			return
		}
		slog.Error("Failed to fetch chairs", err)
		return
	}

	if len(chairs) == 0 {
		slog.Warn("No available chairs")
		return
	}

	// マッチング処理
	for i, ride := range rides {
		if i >= len(chairs) {
			break // 椅子が足りなくなったら終了
		}

		chair := chairs[i]

		// ライドに椅子を割り当てる
		if _, err := database().ExecContext(ctx, `
			UPDATE rides SET chair_id = ? WHERE id = ?
		`, chair.ID, ride.ID); err != nil {
			slog.Error("Failed to update ride", err)
			continue
		}
	}
}
