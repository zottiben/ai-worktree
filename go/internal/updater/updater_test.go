package updater

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"1.0.0", "v1.0.0", 0},
		{"v1.2.0", "v1.1.9", 1},
		{"v1.1.9", "v1.2.0", -1},
		{"v2.0.0", "v1.9.9", 1},
		{"v1.0.1", "v1.0.0", 1},
		// Pre-release sorts before its release.
		{"v1.0.0-beta", "v1.0.0", -1},
		{"v1.0.0", "v1.0.0-beta", 1},
		{"v1.0.0-alpha", "v1.0.0-beta", -1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	v := parseVersion("v2.3.4-rc.1")
	if v.nums != [3]int{2, 3, 4} {
		t.Errorf("nums = %v, want [2 3 4]", v.nums)
	}
	if v.prerelease != "rc.1" {
		t.Errorf("prerelease = %q, want rc.1", v.prerelease)
	}
}

func TestAssetNameForVersion(t *testing.T) {
	// The version prefix and binary base name must be "awt-v..." so the
	// matcher and the release pipeline agree.
	name := AssetNameForVersion("1.2.3")
	if len(name) < len("awt-v1.2.3") || name[:len("awt-v1.2.3")] != "awt-v1.2.3" {
		t.Errorf("AssetNameForVersion = %q, want awt-v1.2.3 prefix", name)
	}
	if !matchesCurrentPlatformAsset(name) {
		t.Errorf("matchesCurrentPlatformAsset(%q) = false, want true", name)
	}
}
