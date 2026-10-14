package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var checkoutButton = regexp.MustCompile(`(?m)^\s*- button "([^"]+)"[^\n]*\[ref=(e[0-9]+)\]`)
var checkoutAmount = regexp.MustCompile(`([0-9][0-9.,]*)\s*€`)

func runBrowse(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "browse", append([]string{"drive"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("browse %s: %w: %s", strings.Join(args[:1], ""), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func purchaseButton(snapshot, expected string, maxTotal float64) (string, string, error) {
	if strings.Contains(strings.ToLower(expected), "ir al pago") || strings.Contains(strings.ToLower(expected), "go to checkout") {
		return "", "", fmt.Errorf("that button only opens checkout; select the final purchase button")
	}
	for _, match := range checkoutButton.FindAllStringSubmatch(snapshot, -1) {
		label := match[1]
		if label != expected {
			continue
		}
		amounts := checkoutAmount.FindAllStringSubmatch(label, -1)
		if len(amounts) != 1 {
			return "", "", fmt.Errorf("checkout button must show exactly one EUR total")
		}
		number := strings.ReplaceAll(amounts[0][1], ".", "")
		number = strings.ReplaceAll(number, ",", ".")
		total, err := strconv.ParseFloat(number, 64)
		if err != nil || total <= 0 || total > maxTotal {
			return "", "", fmt.Errorf("checkout total exceeds --max-total or cannot be parsed")
		}
		return match[2], label, nil
	}
	return "", "", fmt.Errorf("exact checkout button not found; preview again before confirming")
}

func newGlovoPurchaseCmd(st *state) *cobra.Command {
	var citySlug, expectedButton string
	var maxTotal float64
	var confirm bool
	cmd := &cobra.Command{
		Use:   "purchase <store-id> <store-address-id>",
		Short: "Preview or submit a prepared basket via Glovo's web checkout",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			storeID, err := strconv.Atoi(args[0])
			if err != nil || storeID < 1 {
				return fmt.Errorf("invalid store ID")
			}
			storeAddressID, err := strconv.Atoi(args[1])
			if err != nil || storeAddressID < 1 {
				return fmt.Errorf("invalid store address ID")
			}
			if confirm && (expectedButton == "" || maxTotal <= 0) {
				return fmt.Errorf("--confirm requires --expected-button and positive --max-total")
			}
			cl, err := newGlovoClient(st)
			if err != nil {
				return err
			}
			basket, err := cl.GetBasketByStore(cmd.Context(), storeID, storeAddressID)
			if err != nil {
				return err
			}
			var products []json.RawMessage
			if err := json.Unmarshal(basket["products"], &products); err != nil || len(products) == 0 {
				return fmt.Errorf("basket is empty or unreadable")
			}
			cfg := st.glovo()
			language := cfg.Language
			if language == "" {
				language = "es"
			}
			country := strings.ToLower(cfg.CountryCode)
			if country == "" {
				return fmt.Errorf("set Glovo --country-code first")
			}
			if citySlug == "" {
				return fmt.Errorf("--city-slug is required")
			}
			checkoutURL := &url.URL{Scheme: "https", Host: "glovoapp.com", Path: "/" + url.PathEscape(language) + "/" + url.PathEscape(country) + "/" + url.PathEscape(citySlug) + "/order-summary"}
			checkoutURL.RawQuery = url.Values{"storeId": {args[0]}}.Encode()
			label := fmt.Sprintf("ordercli-glovo-%d", time.Now().UnixNano())
			if _, err := runBrowse(cmd.Context(), "tab", "new", "--label", label, checkoutURL.String()); err != nil {
				return err
			}
			closeTab := true
			defer func() {
				if closeTab {
					_, _ = runBrowse(context.Background(), "tab", "close", label)
				}
			}()
			if _, err := runBrowse(cmd.Context(), "tab", label); err != nil {
				return err
			}
			_, _ = runBrowse(cmd.Context(), "wait", "1200")
			currentURL, err := runBrowse(cmd.Context(), "get", "url")
			if err != nil {
				return err
			}
			if !strings.Contains(currentURL, "/order-summary") {
				return fmt.Errorf("Glovo did not open checkout; verify the basket and browser sign-in")
			}
			snapshot, err := runBrowse(cmd.Context(), "snapshot")
			if err != nil {
				return err
			}
			if !confirm {
				fmt.Fprintf(cmd.OutOrStdout(), "Checkout preview for %d basket item(s). No purchase sent.\n", len(products))
				for _, match := range checkoutButton.FindAllStringSubmatch(snapshot, -1) {
					if checkoutAmount.MatchString(match[1]) {
						fmt.Fprintf(cmd.OutOrStdout(), "Button: %s\n", match[1])
					}
				}
				fmt.Fprintln(cmd.OutOrStdout(), "To submit: --confirm --expected-button '<exact button text>' --max-total <EUR>")
				return nil
			}
			ref, _, err := purchaseButton(snapshot, expectedButton, maxTotal)
			if err != nil {
				return err
			}
			if _, err := runBrowse(cmd.Context(), "tab", label); err != nil {
				return err
			}
			if _, err := runBrowse(cmd.Context(), "click", "@"+ref); err != nil {
				return err
			}
			closeTab = false // A payment challenge may still need the browser tab.
			fmt.Fprintf(cmd.OutOrStdout(), "Checkout button clicked. Browser tab %s remains open for payment verification; check order status in Glovo.\n", label)
			return nil
		},
	}
	cmd.Flags().StringVar(&citySlug, "city-slug", "madrid", "Glovo city URL slug")
	cmd.Flags().StringVar(&expectedButton, "expected-button", "", "exact final checkout button text from preview")
	cmd.Flags().Float64Var(&maxTotal, "max-total", 0, "maximum total to authorize in EUR")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "click the exact checkout button")
	return cmd
}
