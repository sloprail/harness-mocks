package hooks

// PluginContributes reports whether a plugin's hooks join the project's own: it
// must be enabled, and its marketplace one the settings declare. A plugin that
// is disabled, or whose marketplace is not declared, contributes none.
//
// sr:capability plugin-hooks
func PluginContributes(enabled, marketplaceDeclared bool) bool {
	return enabled && marketplaceDeclared
}
