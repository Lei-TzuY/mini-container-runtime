package network

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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

func validateNetworkName(name string) error {
	if name == "" {
		return fmt.Errorf("network name cannot be empty")
	}
	if name == "." || name == ".." || strings.ContainsAny(name, "/\\:") {
		return fmt.Errorf("invalid network name %q: path separators and relative components not allowed", name)
	}
	return nil
}

// AllocateIP allocates a free IP address in subnet CIDR (e.g. "172.20.0.0/24") for containerID.
func (ipam *IPAM) AllocateIP(networkName, cidr, containerID string) (string, error) {
	if err := validateNetworkName(networkName); err != nil {
		return "", err
	}

	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	if cidr == "" {
		cidr = "172.20.0.0/24"
	}

	ip, netObj, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}

	pool, err := ipam.loadPool(networkName, cidr)
	if err != nil {
		return "", err
	}

	// Check if container already has allocated IP
	for allocatedIP, cID := range pool.Allocated {
		if cID == containerID {
			return allocatedIP, nil
		}
	}

	// Compute directed broadcast IP for standard IPv4 subnets (<= /30) to exclude from allocation
	var broadcastIP net.IP
	if ipv4 := ip.To4(); ipv4 != nil {
		ones, bits := netObj.Mask.Size()
		if bits == 32 && ones <= 30 {
			broadcastIP = make(net.IP, 4)
			for i := 0; i < 4; i++ {
				broadcastIP[i] = ipv4[i] | ^netObj.Mask[len(netObj.Mask)-4+i]
			}
		}
	}

	// Iterate through subnet host IPs
	currIP := ip.Mask(netObj.Mask)
	incIP(currIP) // Skip network address (e.g. 172.20.0.0)
	incIP(currIP) // Skip gateway address (e.g. 172.20.0.1)

	for netObj.Contains(currIP) {
		if broadcastIP != nil && currIP.Equal(broadcastIP) {
			break // Do not allocate subnet broadcast address
		}

		ipStr := currIP.String()
		if _, taken := pool.Allocated[ipStr]; !taken {
			pool.Allocated[ipStr] = containerID
			if err := ipam.savePool(networkName, pool); err != nil {
				return "", fmt.Errorf("save IPAM pool for %q: %w", networkName, err)
			}
			return ipStr, nil
		}
		incIP(currIP)
	}

	return "", fmt.Errorf("subnet %s exhausted", cidr)
}

// ReleaseIP frees an allocated IP address.
func (ipam *IPAM) ReleaseIP(networkName, containerID string) error {
	if err := validateNetworkName(networkName); err != nil {
		return err
	}

	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	poolFile := filepath.Join(ipam.dir, networkName+".json")
	data, err := os.ReadFile(poolFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
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

func (ipam *IPAM) loadPool(networkName, cidr string) (*SubnetPool, error) {
	poolFile := filepath.Join(ipam.dir, networkName+".json")
	data, err := os.ReadFile(poolFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &SubnetPool{
				Subnet:    cidr,
				Allocated: make(map[string]string),
			}, nil
		}
		return nil, fmt.Errorf("read IPAM pool %q: %w", networkName, err)
	}

	var pool SubnetPool
	if err := json.Unmarshal(data, &pool); err != nil {
		return nil, fmt.Errorf("parse IPAM pool %q: %w", networkName, err)
	}
	if pool.Allocated == nil {
		pool.Allocated = make(map[string]string)
	}
	return &pool, nil
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
