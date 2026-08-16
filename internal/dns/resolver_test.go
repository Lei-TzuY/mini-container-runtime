package dns

import (
	"strings"
	"testing"
)

func TestGenerateResolvConf(t *testing.T) {
	conf := GenerateResolvConf([]string{"9.9.9.9"}, []string{"internal.domain"})
	if !strings.Contains(conf, "search internal.domain") {
		t.Fatalf("resolv.conf missing search domain: %s", conf)
	}
	if !strings.Contains(conf, "nameserver 9.9.9.9") {
		t.Fatalf("resolv.conf missing nameserver: %s", conf)
	}
}
