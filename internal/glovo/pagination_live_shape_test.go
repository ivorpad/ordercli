package glovo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOrderHistoryAcceptsObjectPaginationCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/customer/orders-list" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"pagination":{"currentLimit":12,"next":{"offset":12}},"orders":[]}`))
	}))
	defer srv.Close()
	client, err := New(Options{BaseURL: srv.URL, AccessToken: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.OrderHistory(context.Background(), 0, 12); err != nil {
		t.Fatalf("Glovo's current pagination shape should decode: %v", err)
	}
}
