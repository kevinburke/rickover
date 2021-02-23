package dbtohttp

import (
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/responses"
)

func Job(j newmodels.Job) responses.Job {
	return responses.Job{
		Name:             j.Name,
		DeliveryStrategy: string(j.DeliveryStrategy),
		Attempts:         j.Attempts,
		Concurrency:      j.Concurrency,
		CreatedAt:        j.CreatedAt,
	}
}

func Jobs(js []newmodels.Job) []responses.Job {
	resp := make([]responses.Job, len(js))
	for i := range js {
		resp[i] = Job(js[i])
	}
	return resp
}
