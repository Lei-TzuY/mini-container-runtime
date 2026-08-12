package network

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"minicontainer/internal/state"
)

// IPAM manages IP address allocation for container bridge networks.
type IPAM struct {
	mu      sync.Mutex
	subnets map[string]*SubnetPool
	dir     string
}

type SubnetPool struct {
	Subnet    string            `json:"subnet"`
	Allocated map[string]string `json:"allocated"` // IP -> ContainerID
}

func DefaultIPAMDir() string {
	return filepath.Join(state.DefaultDir(), "ipam")
}

// NewIPAM initializes IPAM manager.
func NewIPAM() *IPAM {
	dir := DefaultIPAMDir()
	_ = os.MkdirAll(dir, 0755)
	return &IPAM{
		subnets: make(map[string]*SubnetPool),
		dir:     dir,
	}
}

// AllocateIP allocates a free IP address in subnet CIDR (e.g. "172.20.0.0/24") for containerID.
func (ipam *IPAM) AllocateIP(networkName, cidr, containerID string) (string, error) {
	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	if cidr == "" {
		cidr = "172.20.0.0/24"
	}

	ip, netObj, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}

	pool := ipam.loadPool(networkName, cidr)

	// Check if container already has allocated IP
	for allocatedIP, cID := range pool.Allocated {
		if cID == containerID {
			return allocatedIP, nil
		}
	}

	// Iterate through subnet host IPs
	currIP := ip.Mask(netObj.Mask)
	incIP(currIP) // Skip network address (e.g. 172.20.0.0)
	incIP(currIP) // Skip gateway address (e.g. 172.20.0.1)

	for netObj.Contains(currIP) {
		ipStr := currIP.String()
		if _, taken := pool.Allocated[ipStr]; !taken {
			pool.Allocated[ipStr] = containerID
			_ = ipam.savePool(networkName, pool)
			return ipStr, nil
		}
		incIP(currIP)
	}

	return "", fmt.Errorf("subnet %s exhausted", cidr)
}

// ReleaseIP frees an allocated IP address.
func (ipam *IPAM) ReleaseIP(networkName, containerID string) error {
	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	poolFile := filepath.Join(ipam.dir, networkName+".json")
	data, err := os.ReadFile(poolFile)
	if err != nil {
		return nil
	}

	var pool SubnetPool
	if err := json.Unmarshal(data, &pool); err != nil {
		return err
	}

	for ipStr, cID := range pool.Allocated {
		if cID == containerID {
			delete(pool.Allocated, ipStr)
			return ipam.savePool(networkName, &pool)
		}
	}
	return nil
}

func (ipam *IPAM) loadPool(networkName, cidr string) *SubnetPool {
	poolFile := filepath.Join(ipam.dir, networkName+".json")
	data, err := os.ReadFile(poolFile)
	if err == nil {
		var pool SubnetPool
		if err := json.Unmarshal(data, &pool); err == nil && pool.Allocated != nil {
			return &pool
		}
	}
	return &SubnetPool{
		Subnet:    cidr,
		Allocated: make(map[string]string),
	}
}

func (ipam *IPAM) savePool(networkName string, pool *SubnetPool) error {
	poolFile := filepath.Join(ipam.dir, networkName+".json")
	data, err := json.MarshalIndent(pool, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(poolFile, data, 0644)
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
