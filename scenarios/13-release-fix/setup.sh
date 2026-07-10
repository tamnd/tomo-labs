#!/usr/bin/env bash
# A tiny Go module that does not build (TaxRate is undefined until the agent
# writes taxes.go from the fetched rates) and does not pass (Subtotal has a real
# bug). The tax rates live on the web sidecar, so the fix spans the network, an
# edit, a build, and a test.
set -e
W="$1"

cat > "$W/go.mod" <<'MOD'
module tomolab/shop

go 1.21
MOD

cat > "$W/spec.md" <<'SPEC'
# Shop pricing

An order total is computed in three steps:

1. Subtotal = unit price * quantity.
2. Apply the destination region's sales tax with WithTax.
3. Report the total rounded to cents.

Regional tax rates come from http://tomolab-web/taxes.json. Encode them in
taxes.go as a function `TaxRate(region string) float64`.

Reference order for report.txt: 12 widgets at $2.50 each, shipped to "CA".
Write the final total, tax included, to two decimals, to report.txt.
SPEC

cat > "$W/pricing.go" <<'GO'
package main

// Subtotal returns the pre-tax total for qty units at the given unit price.
func Subtotal(unitPrice float64, qty int) float64 {
	// BUG: this adds the quantity instead of multiplying by it.
	return unitPrice + float64(qty)
}

// WithTax applies a tax rate to a subtotal: a rate of 0.085 adds 8.5%.
func WithTax(subtotal, rate float64) float64 {
	return subtotal * (1 + rate)
}
GO

cat > "$W/pricing_test.go" <<'GO'
package main

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestSubtotal(t *testing.T) {
	if got := Subtotal(2.50, 4); !near(got, 10.00) {
		t.Fatalf("Subtotal(2.50, 4) = %v, want 10", got)
	}
}

func TestTaxRateFromCatalog(t *testing.T) {
	// TaxRate must come from the fetched taxes.json: CA is 0.085.
	if got := TaxRate("CA"); !near(got, 0.085) {
		t.Fatalf("TaxRate(\"CA\") = %v, want 0.085", got)
	}
}
GO

cat > "$W/main.go" <<'GO'
package main

import (
	"fmt"
	"os"
)

func main() {
	// Reference order from spec.md: 12 widgets at $2.50 each, shipped to CA.
	subtotal := Subtotal(2.50, 12)
	total := WithTax(subtotal, TaxRate("CA"))
	line := fmt.Sprintf("%.2f\n", total)
	if err := os.WriteFile("report.txt", []byte(line), 0o644); err != nil {
		panic(err)
	}
	fmt.Print(line)
}
GO
