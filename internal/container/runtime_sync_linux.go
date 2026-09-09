//go:build linux

package container

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

const runtimeReadyByte byte = 0xa5

const maxRuntimeSyncPayload = 4096

type runtimeBridgeConfig struct {
	ContainerCIDR string `json:"container_cidr"`
	Gateway       string `json:"gateway"`
}

var runtimeBridgeConfigState struct {
	sync.Mutex
	config runtimeBridgeConfig
	set    bool
}

func defaultRuntimeBridgeConfig() runtimeBridgeConfig {
	return runtimeBridgeConfig{
		ContainerCIDR: "172.20.0.2/24",
		Gateway:       "172.20.0.1",
	}
}

func rememberRuntimeBridgeConfig(config runtimeBridgeConfig) {
	runtimeBridgeConfigState.Lock()
	defer runtimeBridgeConfigState.Unlock()
	runtimeBridgeConfigState.config = config
	runtimeBridgeConfigState.set = true
}

func currentRuntimeBridgeConfig() (runtimeBridgeConfig, bool) {
	runtimeBridgeConfigState.Lock()
	defer runtimeBridgeConfigState.Unlock()
	return runtimeBridgeConfigState.config, runtimeBridgeConfigState.set
}

// releaseBlockedChild commits parent-side runtime setup and carries the bridge
// addressing selected by the parent to the blocked container init. Keeping the
// address in this handshake makes host-side veth/DNAT ownership and child-side
// eth0 configuration share one authority instead of separate hard-coded values.
func releaseBlockedChild(writePipe *os.File) error {
	return releaseBlockedChildWithBridge(writePipe, defaultRuntimeBridgeConfig())
}

func releaseBlockedChildWithBridge(writePipe *os.File, config runtimeBridgeConfig) error {
	if writePipe == nil {
		return fmt.Errorf("runtime sync writer is nil")
	}
	payload, err := json.Marshal(config)
	if err != nil {
		_ = writePipe.Close()
		return fmt.Errorf("marshal runtime bridge config: %w", err)
	}
	if len(payload) == 0 || len(payload) > maxRuntimeSyncPayload {
		_ = writePipe.Close()
		return fmt.Errorf("runtime bridge config payload size %d is invalid", len(payload))
	}

	frame := make([]byte, 3+len(payload))
	frame[0] = runtimeReadyByte
	binary.BigEndian.PutUint16(frame[1:3], uint16(len(payload)))
	copy(frame[3:], payload)
	if _, err := writeAllRuntimeSync(writePipe, frame); err != nil {
		_ = writePipe.Close()
		return err
	}
	_ = writePipe.Close()
	return nil
}

func writeAllRuntimeSync(writePipe *os.File, payload []byte) (int, error) {
	written := 0
	for written < len(payload) {
		n, err := writePipe.Write(payload[written:])
		written += n
		if err != nil {
			return written, fmt.Errorf("write runtime readiness frame: %w", err)
		}
		if n == 0 {
			return written, fmt.Errorf("write runtime readiness frame: %w", io.ErrShortWrite)
		}
	}
	return written, nil
}

// awaitParentReady blocks the re-executed child until the parent explicitly
// commits runtime setup. EOF means the parent disappeared or closed the pipe
// without committing, and therefore fails closed. A complete bridge config is
// remembered process-locally for the later container network setup gate.
func awaitParentReady(readPipe *os.File) error {
	if readPipe == nil {
		return fmt.Errorf("runtime sync reader is nil")
	}
	defer readPipe.Close()

	var header [3]byte
	if _, err := io.ReadFull(readPipe, header[:1]); err != nil {
		return fmt.Errorf("await parent runtime readiness: %w", err)
	}
	if header[0] != runtimeReadyByte {
		return fmt.Errorf("invalid runtime ready byte 0x%02x", header[0])
	}
	if _, err := io.ReadFull(readPipe, header[1:]); err != nil {
		return fmt.Errorf("read runtime bridge config length: %w", err)
	}
	payloadLen := int(binary.BigEndian.Uint16(header[1:]))
	if payloadLen <= 0 || payloadLen > maxRuntimeSyncPayload {
		return fmt.Errorf("runtime bridge config payload size %d is invalid", payloadLen)
	}
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(readPipe, payload); err != nil {
		return fmt.Errorf("read runtime bridge config: %w", err)
	}
	var config runtimeBridgeConfig
	if err := json.Unmarshal(payload, &config); err != nil {
		return fmt.Errorf("decode runtime bridge config: %w", err)
	}
	if config.ContainerCIDR == "" || config.Gateway == "" {
		return fmt.Errorf("runtime bridge config is incomplete")
	}
	rememberRuntimeBridgeConfig(config)
	return nil
}
