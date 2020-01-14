package servertest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	types "github.com/kevinburke/go-types"
	"github.com/kevinburke/rickover/server"
	"github.com/kevinburke/rickover/test"
	"github.com/kevinburke/rickover/test/factory"
)

func BenchmarkEnqueue(b *testing.B) {
	defer test.TearDown(b)
	expiry := time.Now().UTC().Add(5 * time.Minute)
	ejr := &server.EnqueueJobRequest{
		Data:      factory.EmptyData,
		ExpiresAt: types.NullTime{Valid: true, Time: expiry},
	}
	buf := new(bytes.Buffer)
	json.NewEncoder(buf).Encode(ejr)
	b.SetBytes(int64(buf.Len()))
	bits := buf.Bytes()
	factory.CreateJob(b, factory.SampleJob)
	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, _ := http.NewRequest("PUT", "/v1/jobs/echo/random_id", bytes.NewReader(bits))
			req.SetBasicAuth("test", testPassword)
			w := httptest.NewRecorder()
			server.DefaultServer.ServeHTTP(w, req)
			if w.Code != 202 {
				b.Fatalf("incorrect Code: %d (response %s)", w.Code, w.Body.Bytes())
			}
		}
	})
}
