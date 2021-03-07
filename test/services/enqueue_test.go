package services

import (
	"context"
	"testing"
	"time"

	"github.com/kevinburke/go-types"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/services"
	"github.com/kevinburke/rickover/test/factory"
)

func testEnqueue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := factory.CreateJob(t, newmodels.CreateJobParams{})
	_, err := services.Enqueue(ctx, newmodels.DB,
		newParams(factory.RandomId(""), job.Name, time.Now().UTC(), types.NullTime{}, factory.EmptyData))
	if err != nil {
		t.Fatal(err)
	}
}
