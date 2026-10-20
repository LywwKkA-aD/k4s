package logsink

// Hooks for the external logsink_test package, which cannot be internal
// because it imports internal/k8s (and k8s imports logsink).
var (
	EnablePluginCapture = pluginStderr.enable
	SwapStderr          = swapStderr
)
