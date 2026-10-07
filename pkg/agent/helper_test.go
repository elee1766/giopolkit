package agent

import "testing"

func TestUnescape(t *testing.T) {
	cases := map[string]string{
		`Password: `:        "Password: ",
		`a\nb`:              "a\nb",
		`quote \"x\"`:       `quote "x"`,
		`back\\slash`:       `back\slash`,
		`\303\251t\303\251`: "été",
		`trailing\`:         `trailing\`,
	}
	for in, want := range cases {
		if got := unescape(in); got != want {
			t.Errorf("unescape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseLine(t *testing.T) {
	k, s, ok := parseLine("PAM_PROMPT_ECHO_OFF Password: ")
	if !ok || k != PromptSecret || s != "Password: " {
		t.Fatal(k, s, ok)
	}
	k, s, ok = parseLine("PAM_TEXT_INFO Please touch the device.")
	if !ok || k != InfoText || s != "Please touch the device." {
		t.Fatal(k, s, ok)
	}
	if _, _, ok := parseLine("garbage"); ok {
		t.Fatal("garbage parsed")
	}
}
