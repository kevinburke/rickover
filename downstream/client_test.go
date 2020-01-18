package downstream

import (
	"context"
	"encoding/json"

	"github.com/kevinburke/go-types"
)

// Create a new client, then make a request to a downstream service with an
// empty data object.
func Example() {
	client := NewClient("test", "hymanrickover", "http://downstream-server.example.com")
	params := JobParams{
		Data:     json.RawMessage([]byte("{}")),
		Attempts: 3,
	}
	id, _ := types.NewPrefixUUID("job_123")
	client.Job.Post(context.TODO(), "invoice-shipment", &id, &params)
}
