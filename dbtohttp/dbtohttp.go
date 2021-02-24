package dbtohttp

import (
	"github.com/kevinburke/rickover/httptypes"
	"github.com/kevinburke/rickover/newmodels"
)

func Job(j newmodels.Job) httptypes.Job {
	return httptypes.Job{
		Name:             j.Name,
		DeliveryStrategy: string(j.DeliveryStrategy),
		Attempts:         j.Attempts,
		Concurrency:      j.Concurrency,
		CreatedAt:        j.CreatedAt,
	}
}

func Jobs(js []newmodels.Job) []httptypes.Job {
	resp := make([]httptypes.Job, len(js))
	for i := range js {
		resp[i] = Job(js[i])
	}
	return resp
}
