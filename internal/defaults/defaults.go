package defaults

import "strings"

const ConfigEnv = "SUPERVISORD_CONFIG"
const ConfigPath = "/home/agent/state/supervisord.config.bin"

func ConfigPathFrom(flagValue string, getenv func(string) string) string {
	if value := strings.TrimSpace(flagValue); value != "" {
		return value
	}
	if getenv != nil {
		if value := strings.TrimSpace(getenv(ConfigEnv)); value != "" {
			return value
		}
	}
	return ConfigPath
}
