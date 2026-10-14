package glovo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// BasketV2 preserves the basket fields returned by Glovo. The web client uses
// PUT /v2/baskets/{basketId} for cart edits.
type BasketV2 map[string]json.RawMessage

// The web client's BasketSchema strips other response-only fields before PUT.
var basketWriteFields = []string{
	"handlingStrategy", "storeAddressId", "storeCategoryId", "storeId",
	"basketId", "basketVersion", "customerId", "products", "productSuggestions",
	"basketPrice", "mbs", "isPrimeSubscriptionSimulated", "cityCode",
	"usingDhBasket", "operationError", "status",
}

type BasketProduct struct {
	Customizations []any `json:"customizations"`
	IDs            struct {
		ID             string `json:"id"`
		ExternalID     string `json:"externalId"`
		LegacyID       string `json:"legacyId"`
		StoreProductID string `json:"storeProductId"`
	} `json:"ids"`
	Quantity struct {
		Increments int `json:"increments"`
	} `json:"quantity"`
}

func (c *Client) GetBasketByStore(ctx context.Context, storeID, storeAddressID int) (BasketV2, error) {
	var basket BasketV2
	err := c.getJSON(ctx, fmt.Sprintf("v2/baskets/stores/%d", storeID), url.Values{
		"storeAddressId": {strconv.Itoa(storeAddressID)},
	}, &basket)
	return basket, err
}

func (c *Client) PutBasket(ctx context.Context, basket BasketV2) (BasketV2, error) {
	var basketID string
	if err := json.Unmarshal(basket["basketId"], &basketID); err != nil || basketID == "" {
		return nil, fmt.Errorf("basket has no basketId")
	}
	writeBasket := make(BasketV2)
	for _, key := range basketWriteFields {
		if value, ok := basket[key]; ok {
			writeBasket[key] = value
		}
	}
	body, err := json.Marshal(writeBasket)
	if err != nil {
		return nil, err
	}
	u := c.baseURL.ResolveReference(&url.URL{
		Path:    "v2/baskets/" + basketID,
		RawPath: "v2/baskets/" + url.PathEscape(basketID),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{Method: req.Method, URL: req.URL.String(), StatusCode: resp.StatusCode, Body: responseBody}
	}
	var updated BasketV2
	if err := json.Unmarshal(responseBody, &updated); err != nil {
		return nil, fmt.Errorf("update basket: decode JSON: %w", err)
	}
	return updated, nil
}

// AddBasketProduct builds the same minimal product shape used by Glovo's web
// cart. It does not call the API; PutBasket is the explicit mutation step.
func AddBasketProduct(basket BasketV2, product BasketProduct) (BasketV2, error) {
	if product.IDs.ID == "" || product.IDs.StoreProductID == "" || product.Quantity.Increments < 1 {
		return nil, fmt.Errorf("product ID, store product ID, and positive quantity are required")
	}
	var products []json.RawMessage
	if err := json.Unmarshal(basket["products"], &products); err != nil {
		return nil, fmt.Errorf("basket products: %w", err)
	}
	for i, raw := range products {
		var existing BasketProduct
		if err := json.Unmarshal(raw, &existing); err != nil {
			return nil, fmt.Errorf("existing product %d: %w", i, err)
		}
		if existing.IDs.ID == product.IDs.ID && existing.IDs.StoreProductID == product.IDs.StoreProductID && len(existing.Customizations) == 0 && len(product.Customizations) == 0 {
			var full map[string]any
			if err := json.Unmarshal(raw, &full); err != nil {
				return nil, err
			}
			quantity, ok := full["quantity"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("existing product has no quantity")
			}
			quantity["increments"] = existing.Quantity.Increments + product.Quantity.Increments
			products[i], _ = json.Marshal(full)
			basket["products"], _ = json.Marshal(products)
			return basket, nil
		}
	}
	raw, err := json.Marshal(product)
	if err != nil {
		return nil, err
	}
	products = append(products, raw)
	basket["products"], err = json.Marshal(products)
	return basket, err
}
