//go:build linux

// internal/cgroups/cgroups_linux.go
//
// Control Groups (cgroups) — Resource Limits

package cgroups

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const cgroupV2Root = "/sys/fs/cgroup"

type Config struct {
	Name string
	MemoryMax int64
	MemoryHigh int64
	CPUWeight int64
	CPUs float64
	CPUSetCPUs string
	CPUSetMems string
	PidsMax int64
}

func Apply(pid int, cfg Config, debug bool) error {
	if pid <= 0 { return fmt.Errorf("invalid cgroup target PID %d", pid) }
	if err := validateCgroupName(cfg.Name); err != nil { return err }
	if err := validateResourceValues(cfg.MemoryMax, cfg.CPUWeight, cfg.CPUs, cfg.PidsMax); err != nil { return err }
	if cfg.MemoryHigh < 0 { return fmt.Errorf("memory high must be non-negative") }
	cfg.CPUSetCPUs = strings.TrimSpace(cfg.CPUSetCPUs)
	cfg.CPUSetMems = strings.TrimSpace(cfg.CPUSetMems)
	if isV2() {
		if debug { fmt.Println("[cgroup] using cgroup v2 (unified hierarchy)") }
		return applyV2(pid, cfg, debug)
	}
	if cfg.MemoryHigh > 0 { return fmt.Errorf("memory high resource control requires cgroup v2") }
	if cfg.CPUSetCPUs != "" || cfg.CPUSetMems != "" { return fmt.Errorf("cpuset resource controls require cgroup v2") }
	if debug { fmt.Println("[cgroup] using cgroup v1 (legacy hierarchy)") }
	return applyV1(pid, cfg, debug)
}

func Remove(name string, debug bool) {
	if err := validateCgroupName(name); err != nil { if debug { fmt.Printf("[cgroup] refusing cleanup for invalid name %q: %v\n", name, err) }; return }
	if isV2() { removePath(filepath.Join(cgroupV2Root, name), debug); return }
	for _, controller := range []string{"memory", "cpu", "pids"} { removePath(filepath.Join("/sys/fs/cgroup", controller, name), debug) }
}
func removePath(path string, debug bool) { if err := os.Remove(path); err != nil && !os.IsNotExist(err) && debug { fmt.Printf("[cgroup] cleanup %s: %v\n", path, err) } }
func isV2() bool { _, err := os.Stat(filepath.Join(cgroupV2Root, "cgroup.controllers")); return err == nil }

func applyV2(pid int, cfg Config, debug bool) error {
	cgPath := filepath.Join(cgroupV2Root, cfg.Name)
	if err := os.Mkdir(cgPath, 0755); err != nil { if errors.Is(err, os.ErrExist) { return fmt.Errorf("cgroup %s already exists; refusing to reuse stale cgroup", cgPath) }; return fmt.Errorf("mkdir cgroup %s: %w", cgPath, err) }
	success := false
	defer func(){ if !success { removePath(cgPath, debug) } }()
	if err := configureV2(cgPath, pid, cfg, debug); err != nil { return err }
	success = true
	return nil
}

func configureV2(cgPath string, pid int, cfg Config, debug bool) error {
	write := func(file, value string) error { path := filepath.Join(cgPath,file); if err:=os.WriteFile(path,[]byte(value),0644); err!=nil{return fmt.Errorf("write %s: %w",path,err)}; if debug{fmt.Printf("[cgroup v2] %-22s = %s\n",file,value)}; return nil }
	if cfg.MemoryMax > 0 {
		if err:=write("memory.max",strconv.FormatInt(cfg.MemoryMax,10)); err!=nil{return err}
		swapPath:=filepath.Join(cgPath,"memory.swap.max"); if _,err:=os.Stat(swapPath); err==nil { if err:=write("memory.swap.max","0");err!=nil{return err} } else if !errors.Is(err,os.ErrNotExist){return fmt.Errorf("inspect %s: %w",swapPath,err)}
	}
	if cfg.MemoryHigh > 0 { if err:=write("memory.high",strconv.FormatInt(cfg.MemoryHigh,10)); err!=nil{return err} }
	if cfg.CPUWeight > 0 { if err:=write("cpu.weight",strconv.FormatInt(cfg.CPUWeight,10));err!=nil{return err} }
	if cfg.CPUs > 0 { periodUs:=int64(100000); quotaUs:=int64(cfg.CPUs*float64(periodUs)); if err:=write("cpu.max",fmt.Sprintf("%d %d",quotaUs,periodUs));err!=nil{return err} }
	if cfg.CPUSetMems!="" { if err:=write("cpuset.mems",cfg.CPUSetMems);err!=nil{return err} }
	if cfg.CPUSetCPUs!="" { if err:=write("cpuset.cpus",cfg.CPUSetCPUs);err!=nil{return err} }
	if cfg.PidsMax>0 { if err:=write("pids.max",strconv.FormatInt(cfg.PidsMax,10));err!=nil{return err} }
	return write("cgroup.procs",strconv.Itoa(pid))
}

func applyV1(pid int, cfg Config, debug bool) error { return applyV1At("/sys/fs/cgroup", pid, cfg, debug) }
