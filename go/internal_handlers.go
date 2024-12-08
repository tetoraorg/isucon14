package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
)

// このAPIをインスタンス内から一定間隔で叩かせることで、椅子とライドをマッチングさせる
func internalGetMatching(ctx context.Context) {
	// MEMO: 一旦最も待たせているリクエストに適当な空いている椅子マッチさせる実装とする。おそらくもっといい方法があるはず…
	ride := &Ride{}
	if err := db.GetContext(ctx, ride, `
	SELECT * 
	FROM rides
	WHERE chair_id IS NULL
	ORDER BY created_at
	LIMIT 1
`); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			slog.Info("no rides")
			return
		}
		slog.Error("Failed to fetch ride", err)
		return
	}

	matched := &Chair{}
	// 「空いている椅子」を一度のクエリで取得する例
	if err := db.GetContext(ctx, matched, `
	SELECT c.*
	FROM chairs c
	WHERE c.is_active = TRUE
	  AND NOT EXISTS (
	    SELECT 1
	    FROM rides r
	    JOIN ride_statuses rs ON r.id = rs.ride_id
	    WHERE r.chair_id = c.id
	    GROUP BY r.id
	    HAVING COUNT(rs.chair_sent_at) <> 6
	  )
	ORDER BY RAND()
	LIMIT 1
`); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// 空いている椅子が無い場合はマッチングできない
			slog.Info("no available chairs")
			return
		}
		slog.Error("Failed to fetch empty chair", err)
		return
	}

	// 椅子が取得できたのでライドに割り当て
	if _, err := db.ExecContext(ctx, `
	UPDATE rides
	SET chair_id = ?
	WHERE id = ?
`, matched.ID, ride.ID); err != nil {
		slog.Error("Failed to update ride", err)
		return
	}

	slog.Info("Successfully matched ride with chair", slog.Any("ride_id", ride.ID), slog.Any("chair_id", matched.ID))

}
