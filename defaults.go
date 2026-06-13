package supervisor

import "strings"

const ConfigEnv = "SUPERVISORD_CONFIG"
const DefaultConfigPath = "/home/agent/state/supervisord.config.bin"

func ConfigPath(flagValue string, getenv func(string) string) string {
	if value := strings.TrimSpace(flagValue); value != "" {
		return value
	}
	if getenv != nil {
		if value := strings.TrimSpace(getenv(ConfigEnv)); value != "" {
			return value
		}
	}
	return DefaultConfigPath
}
