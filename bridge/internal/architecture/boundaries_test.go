package architecture_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/RCooLeR/omada_exporter"

// Production boundaries keep controller protocols behind the Omada facade and
// runtime wiring out of the CLI. Tests may still construct lower-level fixtures.
func TestProductionImportBoundaries(t *testing.T) {
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(moduleRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != moduleRoot && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor" || entry.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(moduleRoot, path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		sourcePackage := filepath.ToSlash(filepath.Dir(relative))
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if violation := importBoundaryViolation(sourcePackage, imported); violation != "" {
				t.Errorf("%s imports %s: %s", filepath.ToSlash(relative), imported, violation)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func importBoundaryViolation(sourcePackage, imported string) string {
	dependency, local := strings.CutPrefix(imported, modulePath+"/")
	if !local {
		return ""
	}
	for _, retired := range []string{"internal/api", "internal/webapi", "internal/openapi"} {
		if withinPackage(dependency, retired) {
			return "retired protocol package; use the Omada facade"
		}
	}
	if (withinPackage(sourcePackage, "internal/collectors") || withinPackage(sourcePackage, "internal/hamqtt")) && withinPackage(dependency, "internal/omada/session") {
		return "metric and MQTT consumers must not depend on controller session internals"
	}
	if withinPackage(sourcePackage, "cmd") && strings.HasPrefix(dependency, "internal/") && dependency != "internal/app" && dependency != "internal/config" {
		return "CLI may depend only on internal/app and internal/config"
	}
	if withinPackage(sourcePackage, "internal/omada/session") {
		if dependency == "internal/omada" {
			return "session layer must not depend on its parent facade"
		}
		for _, consumer := range []string{"internal/app", "internal/collectors", "internal/hamqtt", "internal/debugdump", "cmd"} {
			if withinPackage(dependency, consumer) {
				return "session layer must not depend on its consumers"
			}
		}
	}
	// Raw diagnostic capture intentionally uses the session request interface.
	return ""
}

func withinPackage(path, parent string) bool {
	return path == parent || strings.HasPrefix(path, parent+"/")
}

func TestImportBoundaryRules(t *testing.T) {
	for _, test := range []struct {
		name, source, dependency string
		forbidden                bool
	}{
		{"retired API", "internal/app", "internal/api", true},
		{"retired Web API", "internal/omada", "internal/webapi", true},
		{"retired OpenAPI", "internal/debugdump", "internal/openapi", true},
		{"collector session", "internal/collectors", "internal/omada/session", true},
		{"MQTT session", "internal/hamqtt", "internal/omada/session", true},
		{"CLI runtime internals", "cmd", "internal/collectors", true},
		{"CLI runtime entry", "cmd", "internal/app", false},
		{"CLI configuration", "cmd", "internal/config", false},
		{"session facade", "internal/omada/session", "internal/omada", true},
		{"session consumer", "internal/omada/session", "internal/hamqtt", true},
		{"session configuration", "internal/omada/session", "internal/config", false},
		{"collector facade", "internal/collectors", "internal/omada", false},
		{"diagnostic session", "internal/debugdump", "internal/omada/session", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			violation := importBoundaryViolation(test.source, modulePath+"/"+test.dependency)
			if (violation != "") != test.forbidden {
				t.Fatalf("boundary violation = %q, want forbidden = %v", violation, test.forbidden)
			}
		})
	}
}
