package config

import "testing"

func TestMetaTubeConfiguration(t *testing.T) {
	for _, address := range []string{"", "http://metatube:8080", "https://example.com/prefix/"} {
		cfg, err := Parse([]byte("metatube:\n  url: '" + address + "'\n"))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.MetaTube.TimeoutSeconds != 15 {
			t.Fatal("missing default timeout")
		}
	}
	for _, yaml := range []string{
		"metatube:\n  url: 'file:///tmp/test'",
		"metatube:\n  url: 'https://user:pass@example.com'",
		"metatube:\n  url: 'https://example.com?token=secret'",
		"metatube:\n  timeout_seconds: -1",
		"metatube:\n  timeout_seconds: 121",
	} {
		if _, err := Parse([]byte(yaml)); err == nil {
			t.Fatalf("accepted %s", yaml)
		}
	}
	if !hasRestartRequiredChange([]byte("metatube:\n  url: ''"), []byte("metatube:\n  url: 'http://metatube:8080'")) {
		t.Fatal("endpoint change must require restart")
	}
}
