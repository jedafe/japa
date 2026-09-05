package universities

import "testing"

func TestDeriveInitials(t *testing.T) {
	cases := []struct {
		name     string
		domains  []string
		webPages []string
		want     string
	}{
		{"University of Lagos", []string{"unilag.edu.ng"}, nil, "UNILAG"},
		{"Delta State University", []string{"delsu.edu.ng"}, nil, "DELSU"},
		{"University of the People", []string{"uopeople.edu"}, nil, "UoPeople"},
		{"Massachusetts Institute of Technology", []string{"mit.edu"}, nil, "MIT"},
		{"California Institute of Technology", []string{"caltech.edu"}, nil, "Caltech"},
		{"University of Ibadan", nil, []string{"https://www.ui.edu.ng"}, "UI"},
		{"Obafemi Awolowo University", []string{"oauife.edu.ng"}, nil, "OAU"},
		{"Obafemi Awolowo University, Ile-Ife", nil, nil, "OAU"},
		{"University of Nigeria, Nsukka", []string{"unn.edu.ng"}, nil, "UNN"},
		{"University of Cambridge", []string{"cam.ac.uk"}, nil, "Cambridge"}, // name override
		{"Some Unknown College", nil, nil, "SUC"},                            // fallback name initials
	}

	for _, tc := range cases {
		got := DeriveInitials(tc.name, tc.domains, tc.webPages)
		if got != tc.want {
			t.Errorf("DeriveInitials(%q, %v, %v) = %q; want %q", tc.name, tc.domains, tc.webPages, got, tc.want)
		}
	}
}
