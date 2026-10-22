package feeds

import (
	"os"
	"strings"
	"testing"
)

func TestParseInADS(t *testing.T) {
	for _, tt := range []struct {
		name    string
		in      string
		want    []Address
		skipped int
		err     bool
	}{
		{"empty", `{"addresses":[]}`, nil, 0, false},
		{"bad json", `{`, nil, 0, true},
		{
			"harju first, bad rows skipped",
			`{"addresses":[
				{"aadresstekst":"Pärnu mnt 20","asustusyksus":"Sindi linn","omavalitsus":"Tori vald","maakond":"Pärnu maakond","viitepunkt_b":"58.4","viitepunkt_l":"24.6"},
				{"aadresstekst":"Pärnu mnt 20","asustusyksus":"Kesklinna linnaosa","omavalitsus":"Tallinn","maakond":"Harju maakond","viitepunkt_b":"59.43","viitepunkt_l":"24.74"},
				{"aadresstekst":"Nowhere","viitepunkt_b":"","viitepunkt_l":""}
			]}`,
			[]Address{
				{Label: "Pärnu mnt 20", Detail: "Kesklinna linnaosa, Tallinn", Lat: 59.43, Lon: 24.74},
				{Label: "Pärnu mnt 20", Detail: "Sindi linn, Tori vald", Lat: 58.4, Lon: 24.6},
			},
			1, false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, skipped, err := ParseInADS(strings.NewReader(tt.in))
			if (err != nil) != tt.err || skipped != tt.skipped || len(got) != len(tt.want) {
				t.Fatalf("got %v, %d skipped, err %v", got, skipped, err)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseInADSFixture(t *testing.T) {
	f, err := os.Open("testdata/inads_parnu_mnt_20.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, skipped, err := ParseInADS(f)
	if err != nil || skipped != 0 || len(got) < 3 {
		t.Fatalf("got %d, %d skipped, %v", len(got), skipped, err)
	}
	if got[0].Label != "Pärnu mnt 20" || got[0].Detail != "Kesklinna linnaosa, Tallinn" || got[0].Lat < 59.4 || got[0].Lon < 24.7 {
		t.Errorf("first = %+v", got[0])
	}
}
