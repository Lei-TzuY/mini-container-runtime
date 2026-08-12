package metrics

import (
	"fmt"
	"strings"

	"minicontainer/internal/state"
)

// GeneratePrometheusMetrics generates Prometheus text exposition format metrics from state store.
func GeneratePrometheusMetrics(st *state.Store) (string, error) {
	if st == nil {
		return "", fmt.Errorf("state store is nil")
	}

	ctrs, err := st.List()
	if err != nil {
		return "", err
	}

	imgs, err := st.ListImages()
	if err != nil {
		return "", err
	}

	var sb strings.Builder

	sb.WriteString("# HELP minictl_container_info Container general metadata\n")
	sb.WriteString("# TYPE minictl_container_info gauge\n")
	for _, c := range ctrs {
		sb.WriteString(fmt.Sprintf("minictl_container_info{id=%q,hostname=%q,status=%q,health=%q} 1\n",
			c.ID, c.Hostname, c.Status, c.Health))
	}

	sb.WriteString("\n# HELP minictl_container_status Container state (1 if running, 0 otherwise)\n")
	sb.WriteString("# TYPE minictl_container_status gauge\n")
	for _, c := range ctrs {
		val := 0
		if c.Status == state.StatusRunning {
			val = 1
		}
		sb.WriteString(fmt.Sprintf("minictl_container_status{id=%q} %d\n", c.ID, val))
	}

	sb.WriteString("\n# HELP minictl_container_exit_code Exit code of the container process\n")
	sb.WriteString("# TYPE minictl_container_exit_code gauge\n")
	for _, c := range ctrs {
		sb.WriteString(fmt.Sprintf("minictl_container_exit_code{id=%q} %d\n", c.ID, c.ExitCode))
	}

	sb.WriteString("\n# HELP minictl_images_total Total registered container rootfs images\n")
	sb.WriteString("# TYPE minictl_images_total gauge\n")
	sb.WriteString(fmt.Sprintf("minictl_images_total %d\n", len(imgs)))

	return sb.String(), nil
}
