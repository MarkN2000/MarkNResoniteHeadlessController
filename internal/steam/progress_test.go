package steam

import "testing"

func TestDetectPrompt(t *testing.T) {
	cases := []struct {
		tail string
		want promptKind
	}{
		{`Enter account password for "spareacct": `, promptPassword},
		{`Please enter your 2 factor auth code from your authenticator app: `, promptTwoFactor},
		{`Please enter the authentication code sent to your email address: `, promptTwoFactor},
		{`Using app branch: 'headless'.`, promptNone},
		{``, promptNone},
		{` 12.34% /Resonite/Foo`, promptNone},
	}
	for _, c := range cases {
		if got := detectPrompt(c.tail); got != c.want {
			t.Errorf("detectPrompt(%q)=%d want %d", c.tail, got, c.want)
		}
	}
}

func TestParseProgress(t *testing.T) {
	pct, file, ok := parseProgress(` 12.34% /Resonite/Headless/Foo.dll`)
	if !ok || pct != 12.34 || file != "/Resonite/Headless/Foo.dll" {
		t.Errorf("got (%v,%q,%v)", pct, file, ok)
	}
	pct, _, ok = parseProgress(`100.00% /a`)
	if !ok || pct != 100.0 {
		t.Errorf("100%% got (%v,%v)", pct, ok)
	}
	if _, _, ok := parseProgress(`Using app branch: 'headless'.`); ok {
		t.Error("非進捗行を進捗と誤判定")
	}
	if _, _, ok := parseProgress(`Total downloaded: 100 bytes`); ok {
		t.Error("Total 行を進捗と誤判定")
	}
}

func TestDetectMilestone(t *testing.T) {
	cases := map[string]string{
		"Using app branch: 'headless'.":                                "Using app branch",
		"Downloading depot 2519832":                                    "Downloading depot",
		"Pre-allocating /Resonite/Big.pak":                             "Pre-allocating",
		"Validating /Resonite/Foo":                                     "Validating",
		"Total downloaded: 100 bytes (100 bytes uncompressed) from 1 ": "Total downloaded",
	}
	for line, want := range cases {
		got, ok := detectMilestone(line)
		if !ok || got != want {
			t.Errorf("detectMilestone(%q)=(%q,%v) want %q", line, got, ok, want)
		}
	}
	if _, ok := detectMilestone(" 12.34% /a"); ok {
		t.Error("進捗行をマイルストーンと誤判定")
	}
}
