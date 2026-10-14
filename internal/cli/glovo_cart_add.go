package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/steipete/ordercli/internal/glovo"
)

func newGlovoCartAddCmd(st *state) *cobra.Command {
	var storeProductID, externalID, customizationsPath string
	var quantity int
	var confirm bool
	cmd := &cobra.Command{
		Use:   "add <store-id> <store-address-id> <product-id>",
		Short: "Add a product to a Glovo basket (preview unless --confirm)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			storeID, err := strconv.Atoi(args[0])
			if err != nil || storeID < 1 {
				return fmt.Errorf("invalid store ID")
			}
			storeAddressID, err := strconv.Atoi(args[1])
			if err != nil || storeAddressID < 1 {
				return fmt.Errorf("invalid store address ID")
			}
			if args[2] == "" || quantity < 1 {
				return fmt.Errorf("product ID and positive quantity are required")
			}
			if storeProductID == "" {
				storeProductID = args[2]
			}
			cl, err := newGlovoClient(st)
			if err != nil {
				return err
			}
			basket, err := cl.GetBasketByStore(cmd.Context(), storeID, storeAddressID)
			if err != nil {
				return err
			}
			product := glovo.BasketProduct{Customizations: []any{}}
			product.IDs.ID = args[2]
			product.IDs.LegacyID = args[2]
			product.IDs.ExternalID = externalID
			product.IDs.StoreProductID = storeProductID
			product.Quantity.Increments = quantity
			if customizationsPath != "" {
				b, err := os.ReadFile(customizationsPath)
				if err != nil {
					return err
				}
				if err := json.Unmarshal(b, &product.Customizations); err != nil {
					return fmt.Errorf("customizations JSON: %w", err)
				}
			}
			basket, err = glovo.AddBasketProduct(basket, product)
			if err != nil {
				return err
			}
			if !confirm {
				fmt.Fprintf(cmd.OutOrStdout(), "Preview: add %dx product %s to store %d. No change sent. Re-run with --confirm.\n", quantity, args[2], storeID)
				return nil
			}
			if _, err := cl.PutBasket(cmd.Context(), basket); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "product added to basket")
			return nil
		},
	}
	cmd.Flags().StringVar(&storeProductID, "store-product-id", "", "store-specific product ID (defaults to product ID)")
	cmd.Flags().StringVar(&externalID, "external-id", "", "external product ID")
	cmd.Flags().StringVar(&customizationsPath, "customizations-file", "", "JSON array of selected customizations")
	cmd.Flags().IntVar(&quantity, "quantity", 1, "number of units to add")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "send the basket update to Glovo")
	return cmd
}
