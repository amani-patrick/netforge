package network

import (
	"sync"
)

// DeviceConfig holds running and startup configuration lines per device.
type DeviceConfig struct {
	Running []string `json:"running"`
	Startup []string `json:"startup"`
}

// ConfigStore tracks IOS-style running/startup configs.
type ConfigStore struct {
	devices map[string]*DeviceConfig
	mu      sync.RWMutex
}

// NewConfigStore creates an empty config store.
func NewConfigStore() *ConfigStore {
	return &ConfigStore{devices: make(map[string]*DeviceConfig)}
}

func (c *ConfigStore) getOrCreate(deviceID string) *DeviceConfig {
	if c.devices[deviceID] == nil {
		c.devices[deviceID] = &DeviceConfig{Running: []string{}, Startup: []string{}}
	}
	return c.devices[deviceID]
}

// AppendRunning adds a line to running-config.
func (c *ConfigStore) AppendRunning(deviceID, line string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cfg := c.getOrCreate(deviceID)
	cfg.Running = append(cfg.Running, line)
}

// GetRunning returns running-config lines.
func (c *ConfigStore) GetRunning(deviceID string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cfg := c.devices[deviceID]
	if cfg == nil {
		return []string{}
	}
	out := make([]string, len(cfg.Running))
	copy(out, cfg.Running)
	return out
}

// GetStartup returns startup-config lines.
func (c *ConfigStore) GetStartup(deviceID string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cfg := c.devices[deviceID]
	if cfg == nil {
		return []string{}
	}
	out := make([]string, len(cfg.Startup))
	copy(out, cfg.Startup)
	return out
}

// WriteMemory copies running-config to startup-config.
func (c *ConfigStore) WriteMemory(deviceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cfg := c.getOrCreate(deviceID)
	cfg.Startup = make([]string, len(cfg.Running))
	copy(cfg.Startup, cfg.Running)
}

// SetRunning replaces running-config entirely.
func (c *ConfigStore) SetRunning(deviceID string, lines []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cfg := c.getOrCreate(deviceID)
	cfg.Running = make([]string, len(lines))
	copy(cfg.Running, lines)
}

// PortIDs returns configured interface names on a router.
func (r *Router) PortIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ports := make([]string, 0, len(r.Interfaces))
	for p := range r.Interfaces {
		ports = append(ports, p)
	}
	return ports
}

// PortIDs returns registered switch ports.
func (s *Switch) PortIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ports := make([]string, 0, len(s.Ports))
	for p := range s.Ports {
		ports = append(ports, p)
	}
	return ports
}

// PortIDs returns ASA interface names.
func (a *ASAFirewall) PortIDs() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	ports := make([]string, 0, len(a.Interfaces))
	for p := range a.Interfaces {
		ports = append(ports, p)
	}
	return ports
}

// GetRunningConfig returns running-config for a device.
func (m *Manager) GetRunningConfig(deviceID string) []string {
	if m.configStore == nil {
		return nil
	}
	return m.configStore.GetRunning(deviceID)
}

// GetStartupConfig returns startup-config for a device.
func (m *Manager) GetStartupConfig(deviceID string) []string {
	if m.configStore == nil {
		return nil
	}
	return m.configStore.GetStartup(deviceID)
}

// AppendRunningConfig records a config line.
func (m *Manager) AppendRunningConfig(deviceID, line string) {
	if m.configStore != nil {
		m.configStore.AppendRunning(deviceID, line)
	}
}

// WriteMemory copies running to startup config.
func (m *Manager) WriteMemory(deviceID string) {
	if m.configStore != nil {
		m.configStore.WriteMemory(deviceID)
	}
	m.LogEvent(EventProtocol, deviceID, "", "write memory", nil)
}
