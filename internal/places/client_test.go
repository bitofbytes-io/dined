package places

import "testing"

func TestPriceLevelNumber(t *testing.T) {
	if got := PriceLevelNumber("PRICE_LEVEL_MODERATE"); got != 2 {
		t.Fatalf("got %d", got)
	}
	if got := PriceLevelNumber("PRICE_LEVEL_UNSPECIFIED"); got != 0 {
		t.Fatalf("got %d", got)
	}
}
