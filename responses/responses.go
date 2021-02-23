package responses

import "time"

type Job struct {
	Name             string    `json:"name"`
	DeliveryStrategy string    `json:"delivery_strategy"`
	Attempts         int16     `json:"attempts"`
	Concurrency      int16     `json:"concurrency"`
	CreatedAt        time.Time `json:"created_at"`
}
