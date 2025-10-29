package reservations

import (
	"github.com/Jeomhps/projet-IAC/api-go/internal/handlers/common"
)

// Thin wrappers around shared common helpers to keep imports stable for this package
// while avoiding code duplication. KISS: just delegate.

// Re-export InventoryHost so existing code keeps using reservations.InventoryHost.
type InventoryHost = common.InventoryHost

func WriteTempInventory(hosts []InventoryHost) (string, func(), error) {
	return common.WriteTempInventory(hosts)
}

func BuildAnsibleArgs(level string, forks int, inventoryPath, playbook, extraVars string) []string {
	return common.BuildAnsibleArgs(level, forks, inventoryPath, playbook, extraVars)
}

func EnvForks(def int) int {
	return common.EnvForks(def)
}

func SafeInvVal(s string) string {
	return common.SafeInvVal(s)
}

func SanitizeInventory(inv string) string {
	return common.SanitizeInventory(inv)
}

func LogLevel() string {
	return common.LogLevel()
}

func StreamAnsible(level string) bool {
	return common.StreamAnsible(level)
}

func AnsibleVerbosityFlags(level string) []string {
	return common.AnsibleVerbosityFlags(level)
}

func HashSHA512Crypt(password string) (string, error) {
	return common.HashSHA512Crypt(password)
}
