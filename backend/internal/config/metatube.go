package config

import (
	"errors"
	"net/url"
	"strings"
)

// MetaTube is an optional metadata service used before local frame extraction.
type MetaTube struct {
	URL            string `yaml:"url"`
	Token          string `yaml:"token"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

func (m *MetaTube) Validate() error {
	m.URL = strings.TrimRight(strings.TrimSpace(m.URL), "/")
	m.Token = strings.TrimSpace(m.Token)
	if m.TimeoutSeconds == 0 {
		m.TimeoutSeconds = 15
	}
	if m.TimeoutSeconds < 1 || m.TimeoutSeconds > 120 {
		return errors.New("metatube.timeout_seconds must be between 1 and 120")
	}
	if m.URL == "" {
		return nil
	}
	u, err := url.Parse(m.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("metatube.url must be an HTTP(S) service URL without credentials, query or fragment")
	}
	return nil
}
