package feeds

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// DefaultInADSURL is the Estonian Land Board's address search (In-ADS gazetteer).
const DefaultInADSURL = "https://inaadress.maaamet.ee/inaadress/gazetteer"

// Address is a geocoded Estonian address.
type Address struct {
	Label  string  `json:"label"`  // "Pärnu mnt 20"
	Detail string  `json:"detail"` // "Kesklinna linnaosa, Tallinn"
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
}

// ParseInADS reads an In-ADS gazetteer response. Rows without coordinates are
// skipped and counted; Harju county (Tallinn and around) comes first.
func ParseInADS(r io.Reader) (out []Address, skipped int, err error) {
	var doc struct {
		Addresses []struct {
			Text     string `json:"aadresstekst"`
			Locality string `json:"asustusyksus"`
			Muni     string `json:"omavalitsus"`
			County   string `json:"maakond"`
			Lat      string `json:"viitepunkt_b"`
			Lon      string `json:"viitepunkt_l"`
		} `json:"addresses"`
	}
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return nil, 0, fmt.Errorf("in-ads: %w", err)
	}
	var harju []bool
	for _, a := range doc.Addresses {
		lat, err1 := strconv.ParseFloat(a.Lat, 64)
		lon, err2 := strconv.ParseFloat(a.Lon, 64)
		if err1 != nil || err2 != nil || a.Text == "" || lat == 0 || lon == 0 {
			skipped++
			continue
		}
		var detail []string
		for _, p := range []string{a.Locality, a.Muni} {
			if p != "" && !slices.Contains(detail, p) {
				detail = append(detail, p)
			}
		}
		if len(detail) == 0 && a.County != "" {
			detail = append(detail, a.County)
		}
		out = append(out, Address{Label: a.Text, Detail: strings.Join(detail, ", "), Lat: lat, Lon: lon})
		harju = append(harju, a.County == "Harju maakond")
	}
	// Stable: keep the gazetteer's own ranking within each group.
	idx := make([]int, len(out))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int {
		switch {
		case harju[a] && !harju[b]:
			return -1
		case harju[b] && !harju[a]:
			return 1
		}
		return 0
	})
	sorted := make([]Address, len(out))
	for i, j := range idx {
		sorted[i] = out[j]
	}
	return sorted, skipped, nil
}
