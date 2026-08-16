package dns

import (
	"fmt"
	"strings"
)

// GenerateResolvConf formats custom nameserver IPs and search domain rules.
func GenerateResolvConf(nameservers []string, searchDomains []string) string {
	var lines []string

	if len(searchDomains) > 0 {
		lines = append(lines, fmt.Sprintf("search %s", strings.Join(searchDomains, " ")))
	}

	for _, ns := range nameservers {
		lines = append(lines, fmt.Sprintf("nameserver %s", ns))
	}

	if len(nameservers) == 0 {
		lines = append(lines, "nameserver 1.1.1.1", "nameserver 8.8.8.8")
	}

	return strings.Join(lines, "\n") + "\n"
}
