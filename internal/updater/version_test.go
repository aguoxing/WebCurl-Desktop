package updater

import "testing"

func TestVersionNewer(t *testing.T) {
	cases := []struct {
		tag, current string
		want         bool
	}{
		{"1.0.1", "1.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"0.9.9", "1.0.0", false},
		{"2.0", "1.9.9", true},
		{"1.0.1", "", true},
		{"", "1.0.0", false},
	}

	for _, tc := range cases {
		if got := versionNewer(tc.tag, tc.current); got != tc.want {
			t.Errorf("versionNewer(%q, %q) = %v, want %v", tc.tag, tc.current, got, tc.want)
		}
	}
}
