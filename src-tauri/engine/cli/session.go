package cli

import (
	"fmt"
	"strings"

	"netforge/engine/network"
	"netforge/engine/pdu"
)

// ConfigMode is the IOS configuration context.
type ConfigMode string

const (
	ModeExec            ConfigMode = "exec"
	ModePrivileged      ConfigMode = "privileged"
	ModeConfig          ConfigMode = "config"
	ModeConfigIf        ConfigMode = "config-if"
	ModeConfigRtr       ConfigMode = "config-router"
	ModeConfigClassMap  ConfigMode = "config-class-map"
	ModeConfigPolicyMap ConfigMode = "config-policy-map"
	ModeConfigPolicyCls ConfigMode = "config-policy-class"
)

// Session tracks CLI state for one device.
type Session struct {
	DeviceID    string
	Mode        ConfigMode
	Iface       string
	RouterProto string
	ClassMapName string
	PolicyMapName string
	PolicyClassName string
}

// SessionStore holds per-device CLI sessions.
type SessionStore struct {
	sessions map[string]*Session
}

// NewSessionStore creates a session store.
func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*Session)}
}

func (s *SessionStore) get(deviceID string) *Session {
	if s.sessions[deviceID] == nil {
		s.sessions[deviceID] = &Session{DeviceID: deviceID, Mode: ModePrivileged}
	}
	return s.sessions[deviceID]
}

// GetSession returns the CLI session for a device.
func (s *SessionStore) GetSession(deviceID string) *Session {
	return s.get(deviceID)
}

// ExecutorWithSession extends Executor with config mode support.
type ExecutorWithSession struct {
	Mgr      *network.Manager
	Sessions *SessionStore
}

// Execute runs a command respecting session mode.
func (e *ExecutorWithSession) Execute(deviceID, line string) Result {
	line = strings.TrimSpace(line)
	if line == "" {
		return Result{Success: true}
	}

	sess := e.Sessions.get(deviceID)
	parts := strings.Fields(line)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "enable":
		sess.Mode = ModePrivileged
		return Result{Success: true}
	case "configure", "conf":
		if len(parts) >= 2 && (parts[1] == "terminal" || parts[1] == "t") {
			sess.Mode = ModeConfig
			return Result{Lines: []string{"Enter configuration commands, one per line. End with CNTL/Z."}, Success: true}
		}
	case "interface", "int":
		if len(parts) < 2 {
			return Result{Error: "% Incomplete command.", Success: false}
		}
		sess.Mode = ModeConfigIf
		sess.Iface = parts[1]
		return Result{Lines: []string{fmt.Sprintf("Enter interface configuration mode for %s.", sess.Iface)}, Success: true}
	case "router":
		if len(parts) < 2 {
			return Result{Error: "% Incomplete command.", Success: false}
		}
		sess.Mode = ModeConfigRtr
		sess.RouterProto = parts[1]
		return e.applyRouterConfig(deviceID, parts, line)
	case "write":
		if len(parts) >= 2 && parts[1] == "memory" {
			e.Mgr.WriteMemory(deviceID)
			return Result{Lines: []string{"Building configuration...", "[OK]"}, Success: true}
		}
	case "copy":
		if len(parts) >= 4 && parts[1] == "running-config" && parts[2] == "startup-config" {
			e.Mgr.WriteMemory(deviceID)
			return Result{Lines: []string{"Destination filename [startup-config]", "Building configuration...", "[OK]"}, Success: true}
		}
	case "show":
		if r, ok := e.execShowExtended(deviceID, parts); ok {
			return r
		}
		return e.execShow(deviceID, parts)
	case "end":
		sess.Mode = ModePrivileged
		sess.Iface = ""
		sess.RouterProto = ""
		return Result{Lines: []string{"Returning to privileged EXEC mode."}, Success: true}
	case "exit":
		switch sess.Mode {
		case ModeConfigPolicyCls:
			sess.Mode = ModeConfigPolicyMap
			sess.PolicyClassName = ""
			return Result{Success: true}
		case ModeConfigPolicyMap, ModeConfigClassMap:
			sess.Mode = ModeConfig
			sess.PolicyMapName = ""
			sess.ClassMapName = ""
			return Result{Success: true}
		case ModeConfigIf, ModeConfigRtr:
			sess.Mode = ModeConfig
			sess.Iface = ""
			sess.RouterProto = ""
			return Result{Success: true}
		case ModeConfig:
			sess.Mode = ModePrivileged
			return Result{Success: true}
		default:
			sess.Mode = ModeExec
			return Result{Success: true}
		}
	}

	if sess.Mode == ModeConfig || sess.Mode == ModeConfigIf || sess.Mode == ModeConfigRtr ||
		sess.Mode == ModeConfigClassMap || sess.Mode == ModeConfigPolicyMap || sess.Mode == ModeConfigPolicyCls {
		e.Mgr.AppendRunningConfig(deviceID, line)
		if r, ok := e.dispatchConfig(deviceID, sess, parts, line); ok {
			return r
		}
	}

	// Privileged/exec commands
	if r, ok := e.dispatchExtended(deviceID, sess, parts, line); ok {
		return r
	}
	if r, ok := e.dispatchConfig(deviceID, sess, parts, line); ok {
		return r
	}

	base := &Executor{Mgr: e.Mgr}
	result := base.Execute(deviceID, line)
	if result.Success && (sess.Mode == ModeConfig || sess.Mode == ModeConfigIf || sess.Mode == ModeConfigRtr) {
		result.Lines = append([]string{fmt.Sprintf("Applying: %s", line)}, result.Lines...)
	}
	return result
}

func (e *ExecutorWithSession) execShow(deviceID string, parts []string) Result {
	if len(parts) < 2 {
		return Result{Error: "% Incomplete command.", Success: false}
	}
	sub := strings.ToLower(parts[1])
	switch sub {
	case "running-config", "run":
		lines := e.Mgr.GetRunningConfig(deviceID)
		if len(lines) == 0 {
			lines = e.generateRunningConfig(deviceID)
		}
		out := append([]string{"Building configuration...", ""}, lines...)
		out = append(out, "end")
		return Result{Lines: out, Success: true}
	case "startup-config", "start":
		lines := e.Mgr.GetStartupConfig(deviceID)
		if len(lines) == 0 {
			return Result{Lines: []string{"startup-config is not present"}, Success: true}
		}
		out := append([]string{""}, lines...)
		out = append(out, "end")
		return Result{Lines: out, Success: true}
	}
	base := &Executor{Mgr: e.Mgr}
	return base.Execute(deviceID, strings.Join(parts, " "))
}

func (e *ExecutorWithSession) generateRunningConfig(deviceID string) []string {
	router, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return []string{fmt.Sprintf("hostname %s", deviceID)}
	}
	lines := []string{fmt.Sprintf("hostname %s", deviceID)}
	for portID, ip := range router.Interfaces {
		mask := router.InterfaceMask[portID]
		lines = append(lines, fmt.Sprintf("interface %s", portID))
		lines = append(lines, fmt.Sprintf(" ip address %s %s", ip, mask))
	}
	if router.Ospf != nil && router.Ospf.Enabled {
		lines = append(lines, "router ospf 1")
	}
	if router.Rip != nil && router.Rip.Enabled {
		lines = append(lines, "router rip")
	}
	return lines
}

func (e *ExecutorWithSession) applyRouterConfig(deviceID string, parts []string, line string) Result {
	e.Mgr.AppendRunningConfig(deviceID, line)
	base := &Executor{Mgr: e.Mgr}
	return base.Execute(deviceID, line)
}

// ExecuteConfig applies a batch of config lines in order.
func (e *ExecutorWithSession) ExecuteConfig(deviceID string, lines []string) []Result {
	results := make([]Result, 0, len(lines))
	for _, line := range lines {
		results = append(results, e.Execute(deviceID, line))
	}
	return results
}

// ConfigureInterface is a helper for config-if mode commands.
func ConfigureInterface(e *ExecutorWithSession, deviceID, portID, ip, mask string) Result {
	e.Sessions.get(deviceID).Mode = ModeConfigIf
	e.Sessions.get(deviceID).Iface = portID
	line := fmt.Sprintf("ip address %s %s", ip, mask)
	e.Mgr.AppendRunningConfig(deviceID, fmt.Sprintf("interface %s", portID))
	e.Mgr.AppendRunningConfig(deviceID, line)
	router, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{Error: "not a router", Success: false}
	}
	router.AddInterface(portID, pdu.IPAddress(ip), mask, pdu.MACAddress("00:00:00:00:00:01"))
	return Result{Success: true}
}
