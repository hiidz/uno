package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/hiidz/uno/internal/addon"
)

// openAPISpecPath is the OpenAPI document for the server's routes.
const openAPISpecPath = "../../docs/api/openapi.yaml"

// openAPIMethods are the path item keys that name an operation.
var openAPIMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// addonPatternConsts are the addon constants routes() builds patterns from,
// by name.
var addonPatternConsts = map[string]string{
	"ManifestPathPattern":  addon.ManifestPathPattern,
	"ConfigurePathPattern": addon.ConfigurePathPattern,
}

// samplePathValues fill an OpenAPI path's template expressions to make a
// request the router can match; any other expression gets "x".
var samplePathValues = map[string]string{
	"profileIndex":  "1",
	"catalogID":     "00000000-0000-0000-0000-000000000001",
	"collectionID":  "00000000-0000-0000-0000-000000000002",
	"publicationID": "00000000-0000-0000-0000-000000000003",
	"type":          "movie",
	"id":            "1",
	"extra":         "skip=20",
}

var templateExpr = regexp.MustCompile(`\{(\w+)\}`)

// TestOpenAPISpecMatchesRoutes holds docs/api/openapi.yaml to the route
// table: every operation it describes reaches a registered route, and every
// registered route but the fallbacks (apiNotFound and the SPA) is described.
func TestOpenAPISpecMatchesRoutes(t *testing.T) {
	s := newProfileTestServer(t, newTestVaultDB(t))

	documented := map[string]bool{}
	for path, item := range openAPIPaths(t) {
		for _, method := range openAPIMethods {
			if _, ok := item[method]; !ok {
				continue
			}
			r := httptest.NewRequest(strings.ToUpper(method), samplePath(path), nil)
			_, pattern := s.router.Handler(r)
			if slices.Contains(fallbackPatterns, pattern) {
				t.Errorf("%s %s is in the spec but no route serves it", strings.ToUpper(method), path)
				continue
			}
			documented[pattern] = true
		}
	}

	for _, pattern := range registeredPatterns(t) {
		if !documented[pattern] {
			t.Errorf("route %q is not in %s", pattern, openAPISpecPath)
		}
	}
}

// fallbackPatterns are the patterns that answer a request no documented route
// serves: no match, the /api 404 and the SPA.
var fallbackPatterns = []string{"", "/", "/api/"}

// openAPIPaths is the spec's path items by path.
func openAPIPaths(t *testing.T) map[string]map[string]any {
	t.Helper()
	raw, err := os.ReadFile(openAPISpecPath)
	if err != nil {
		t.Fatalf("reading spec: %v", err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parsing spec: %v", err)
	}
	if len(spec.Paths) == 0 {
		t.Fatal("spec has no paths")
	}
	return spec.Paths
}

// samplePath is path with each template expression filled from
// samplePathValues.
func samplePath(path string) string {
	return templateExpr.ReplaceAllStringFunc(path, func(expr string) string {
		if v, ok := samplePathValues[expr[1:len(expr)-1]]; ok {
			return v
		}
		return "x"
	})
}

// registeredPatterns is every pattern routes() in server.go registers on
// s.router, except the fallbackPatterns, read from its source.
func registeredPatterns(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "server.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing server.go: %v", err)
	}
	var patterns []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isRouterRegistration(call) {
			return true
		}
		pattern := patternValue(t, call.Args[0])
		if !slices.Contains(fallbackPatterns, pattern) {
			patterns = append(patterns, pattern)
		}
		return true
	})
	if len(patterns) == 0 {
		t.Fatal("found no route registrations in server.go")
	}
	slices.Sort(patterns)
	return patterns
}

// isRouterRegistration reports whether call is s.router.HandleFunc or
// s.router.Handle.
func isRouterRegistration(call *ast.CallExpr) bool {
	fn, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (fn.Sel.Name != "HandleFunc" && fn.Sel.Name != "Handle") || len(call.Args) == 0 {
		return false
	}
	recv, ok := fn.X.(*ast.SelectorExpr)
	return ok && recv.Sel.Name == "router"
}

// patternValue is the string a route pattern expression evaluates to: string
// literals, joined with +, and the addon constants in addonPatternConsts.
func patternValue(t *testing.T, expr ast.Expr) string {
	t.Helper()
	switch e := expr.(type) {
	case *ast.BasicLit:
		s, err := strconv.Unquote(e.Value)
		if err != nil {
			t.Fatalf("route pattern %s: %v", e.Value, err)
		}
		return s
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			return patternValue(t, e.X) + patternValue(t, e.Y)
		}
	case *ast.SelectorExpr:
		if pkg, ok := e.X.(*ast.Ident); ok && pkg.Name == "addon" {
			if v, ok := addonPatternConsts[e.Sel.Name]; ok {
				return v
			}
		}
	}
	t.Fatalf("route pattern %T in server.go is not one the test can read; extend patternValue", expr)
	return ""
}
