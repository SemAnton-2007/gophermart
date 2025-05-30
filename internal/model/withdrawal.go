package model

import "time"

type Withdrawal struct {
	Order       string
	Sum         float64
	ProcessedAt time.Time
	UserID      int64
}
