package architecture

import (
	"os/exec"
	"strings"
	"testing"
)

// Query-facing DTOs and the filesystem/SQL capability implementations are
// independently importable without the causal Store, writer sinks or input
// ledger mutation ports. The shared engine SQLite adapter still hosts the
// legacy verifier; this gate does not pretend it is a separate package.
func TestF4ReadCapabilityTransitiveDependencies(t *testing.T) {
	const module = "github.com/carloshgalves/cote-ai-simulation/"
	for _, path := range []string{"./internal/csf/observatory", "./internal/localdbguard", "./internal/readonlysql"} {
		cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", path)
		cmd.Dir = "../.."
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatal("dependency inventory failed", err, string(output))
		}
		for _, dependency := range strings.Fields(string(output)) {
			if !strings.HasPrefix(dependency, module) {
				continue
			}
			for _, forbidden := range []string{"internal/csf/ports", "internal/csf/persistence", "internal/csf/commit",
				"internal/csf/inputledger", "internal/csf/ownercheckpoint", "internal/csf/operationalmetrics", "internal/physicalhost"} {
				if dependency == module+forbidden || strings.HasPrefix(dependency, module+forbidden+"/") {
					t.Fatal("query/guard capability imports a writer", path, dependency)
				}
			}
		}
	}
}
