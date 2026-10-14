package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlovoCartAddPreviewDoesNotWriteAndConfirmPreservesBasket(t *testing.T) {
	var puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /v2/baskets/stores/21033":
			if r.URL.Query().Get("storeAddressId") != "347695" {
				t.Errorf("missing store address ID")
			}
			_, _ = w.Write([]byte(`{"basketId":"basket-1","basketVersion":"v1","storeId":21033,"storeAddressId":347695,"products":[],"basketPrice":{"final":{"major":0}},"status":"OPEN"}`))
		case "PUT /v2/baskets/basket-1":
			puts++
			if r.Header.Get("Authorization") != "Bearer token" {
				t.Errorf("missing Glovo token")
			}
			var body struct {
				BasketVersion string `json:"basketVersion"`
				Products      []struct {
					IDs struct {
						ID             string `json:"id"`
						StoreProductID string `json:"storeProductId"`
					} `json:"ids"`
					Quantity struct {
						Increments int `json:"increments"`
					} `json:"quantity"`
				} `json:"products"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode basket: %v", err)
			}
			if body.BasketVersion != "v1" || len(body.Products) != 1 || body.Products[0].IDs.StoreProductID != "sku-99" || body.Products[0].Quantity.Increments != 2 {
				t.Errorf("basket update lost fields or product: %+v", body)
			}
			_, _ = w.Write([]byte(`{"basketId":"basket-1","products":[{}]}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if _, _, err := runCLI(cfgPath, []string{"glovo", "config", "set", "--base-url", srv.URL}, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(cfgPath, []string{"glovo", "session", "token"}, ""); err != nil {
		t.Fatal(err)
	}
	args := []string{"glovo", "cart", "add", "21033", "347695", "product-1", "--store-product-id", "sku-99", "--quantity", "2"}
	out, _, err := runCLI(cfgPath, args, "")
	if err != nil || !strings.Contains(out, "No change sent") || puts != 0 {
		t.Fatalf("preview wrote to Glovo: puts=%d err=%v out=%q", puts, err, out)
	}
	out, _, err = runCLI(cfgPath, append(args, "--confirm"), "")
	if err != nil || puts != 1 || !strings.Contains(out, "product added") {
		t.Fatalf("confirm failed: puts=%d err=%v out=%q", puts, err, out)
	}
}

func TestGlovoPurchaseButtonRequiresExactLabelAndCap(t *testing.T) {
	snapshot := `- button "Confirmar pedido • 28,81 €" [ref=e42]`
	if _, _, err := purchaseButton(`- button "Ir al pago • 28,81 €" [ref=e7]`, "Ir al pago • 28,81 €", 30); err == nil {
		t.Fatal("accepted a button that only opens checkout")
	}
	if _, _, err := purchaseButton(snapshot, "Pagar • 28,81 €", 30); err == nil {
		t.Fatal("accepted a different checkout button")
	}
	if _, _, err := purchaseButton(snapshot, "Confirmar pedido • 28,81 €", 28); err == nil {
		t.Fatal("accepted a checkout above the spending cap")
	}
	ref, _, err := purchaseButton(snapshot, "Confirmar pedido • 28,81 €", 30)
	if err != nil || ref != "e42" {
		t.Fatalf("valid checkout rejected: ref=%q err=%v", ref, err)
	}
}
