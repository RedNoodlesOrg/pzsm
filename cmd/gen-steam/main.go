// gen-steam reads Steam's GetSupportedAPIList JSON dump and emits a Go file
// with one request struct and one *Client receiver method per Steam Web API
// method. The generated code lives in package steam and reuses c.get and
// c.postForm helpers; responses are returned as json.RawMessage because the
// metadata does not include response shapes.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"sort"
	"strings"
	"text/template"
)

type apiList struct {
	APIList struct {
		Interfaces []apiInterface `json:"interfaces"`
	} `json:"apilist"`
}

type apiInterface struct {
	Name    string      `json:"name"`
	Methods []apiMethod `json:"methods"`
}

type apiMethod struct {
	Name        string     `json:"name"`
	Version     int        `json:"version"`
	HTTPMethod  string     `json:"httpmethod"`
	Description string     `json:"description"`
	Parameters  []apiParam `json:"parameters"`
}

type apiParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Optional    bool   `json:"optional"`
	Description string `json:"description"`
}

// initialisms maps lowercase Steam param fragments to Go-idiomatic forms.
// Applied per word after splitting on '_'.
var initialisms = map[string]string{
	"id":               "ID",
	"ids":              "IDs",
	"url":              "URL",
	"urls":             "URLs",
	"api":              "API",
	"appid":            "AppID",
	"steamid":          "SteamID",
	"publishedfileid":  "PublishedFileID",
	"publishedfileids": "PublishedFileIDs",
	"kv":               "KV",
	"kvtags":           "KVTags",
	"ugc":              "UGC",
	"bbcode":           "BBCode",
	"json":             "JSON",
	"xml":              "XML",
	"html":             "HTML",
	"http":             "HTTP",
	"cdn":              "CDN",
	"ip":               "IP",
	"os":               "OS",
}

var goReserved = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

func toGoName(name string) string {
	parts := strings.Split(name, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		if v, ok := initialisms[strings.ToLower(p)]; ok {
			parts[i] = v
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	out := strings.Join(parts, "")
	if goReserved[strings.ToLower(out)] {
		out += "_"
	}
	return out
}

// parseAllowlist returns nil for empty input (meaning "no filter"), otherwise
// a set of allowed interface names. Whitespace and empty entries are dropped.
func parseAllowlist(s string) map[string]bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	out := map[string]bool{}
	for part := range strings.SplitSeq(s, ",") {
		if name := strings.TrimSpace(part); name != "" {
			out[name] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// stripInterfacePrefix turns "IPublishedFileService" into "PublishedFileService"
// and leaves names without the leading "I" untouched (e.g. "icloudservice").
func stripInterfacePrefix(name string) string {
	if len(name) > 1 && name[0] == 'I' && name[1] >= 'A' && name[1] <= 'Z' {
		return name[1:]
	}
	return name
}

type fieldView struct {
	GoName  string
	GoType  string
	Comment string
	Encode  string // ready-to-paste Go statement(s) using `args` as the url.Values
}

type methodView struct {
	StructName  string
	GoName      string
	Path        string
	HTTPMethod  string
	Description string
	Fields      []fieldView
}

// formatExpr returns a Go expression that converts `value` (of the given
// Steam wire type) to its string form for url.Values.Set. uint64 stays as
// string on the Go side.
func formatExpr(t, value string) string {
	switch t {
	case "string", "{enum}", "{message}", "uint64":
		return value
	case "bool":
		return "strconv.FormatBool(" + value + ")"
	case "int32":
		return "strconv.FormatInt(int64(" + value + "), 10)"
	case "uint32":
		return "strconv.FormatUint(uint64(" + value + "), 10)"
	default:
		return value
	}
}

func goTypeFor(t string) string {
	switch t {
	case "string", "{enum}", "{message}", "uint64":
		return "string"
	case "bool":
		return "bool"
	case "int32":
		return "int32"
	case "uint32":
		return "uint32"
	default:
		return "string"
	}
}

func buildField(p apiParam) fieldView {
	wire, isArray := wireName(p.Name)
	base := goTypeFor(p.Type)

	var goType string
	switch {
	case isArray:
		goType = "[]" + base
	case p.Optional:
		goType = "*" + base
	default:
		goType = base
	}

	var commentParts []string
	if p.Optional {
		commentParts = append(commentParts, "optional")
	}
	switch p.Type {
	case "uint64":
		commentParts = append(commentParts, "uint64")
	case "{enum}":
		commentParts = append(commentParts, "enum")
	case "{message}":
		commentParts = append(commentParts, "protobuf message")
	}
	if d := strings.TrimSpace(p.Description); d != "" {
		commentParts = append(commentParts, d)
	}

	return fieldView{
		GoName:  toGoName(wire),
		GoType:  goType,
		Comment: strings.Join(commentParts, "; "),
	}
}

// wireName splits a Steam parameter name into its wire form and whether it's
// a legacy form-indexed array (publishedfileids[0] → "publishedfileids", true).
func wireName(name string) (string, bool) {
	return strings.CutSuffix(name, "[0]")
}

// encodeForField renders the Go statement that writes one request field into
// `args` (the url.Values). Built only after disambiguation so the final goName
// is captured.
func encodeForField(p apiParam, goName string) string {
	wire, isArray := wireName(p.Name)
	switch {
	case isArray:
		return fmt.Sprintf("\tfor i, v := range req.%s {\n\t\targs.Set(fmt.Sprintf(\"%s[%%d]\", i), %s)\n\t}", goName, wire, formatExpr(p.Type, "v"))
	case p.Optional:
		return fmt.Sprintf("\tif req.%s != nil {\n\t\targs.Set(\"%s\", %s)\n\t}", goName, wire, formatExpr(p.Type, "*req."+goName))
	default:
		return fmt.Sprintf("\targs.Set(\"%s\", %s)", wire, formatExpr(p.Type, "req."+goName))
	}
}

func buildMethodView(svc apiInterface, m apiMethod) methodView {
	svcGo := stripInterfacePrefix(svc.Name)
	goMethod := svcGo + m.Name

	mv := methodView{
		StructName:  goMethod + "Request",
		GoName:      goMethod,
		Path:        fmt.Sprintf("/%s/%s/v%d/", svc.Name, m.Name, m.Version),
		HTTPMethod:  m.HTTPMethod,
		Description: strings.TrimSpace(m.Description),
	}

	used := map[string]int{}
	for _, p := range m.Parameters {
		f := buildField(p)
		if n, dup := used[f.GoName]; dup {
			used[f.GoName] = n + 1
			f.GoName = fmt.Sprintf("%s_%d", f.GoName, n+1)
		} else {
			used[f.GoName] = 1
		}
		f.Encode = encodeForField(p, f.GoName)
		mv.Fields = append(mv.Fields, f)
	}
	return mv
}

const fileTmpl = `// Code generated by cmd/gen-steam. DO NOT EDIT.

package {{.Pkg}}

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Suppress unused-import complaints when no method needs a given helper.
var (
	_ = fmt.Sprintf
	_ = strconv.Itoa
	_ = json.RawMessage(nil)
	_ = url.Values(nil)
)
{{range .Methods}}
// {{.StructName}} is the request for {{.HTTPMethod}} {{.Path}}.
type {{.StructName}} struct {
{{range .Fields}}	{{.GoName}} {{.GoType}}{{if .Comment}} // {{.Comment}}{{end}}
{{end}}}

// {{.GoName}} calls {{.HTTPMethod}} {{.Path}}.{{if .Description}}
//
// {{.Description}}{{end}}
func (c *Client) {{.GoName}}(ctx context.Context, req {{.StructName}}) (json.RawMessage, error) {
	args := url.Values{}
{{range .Fields}}{{.Encode}}
{{end}}	var out json.RawMessage
	if err := c.{{if eq .HTTPMethod "GET"}}get{{else}}postForm{{end}}(ctx, "{{.Path}}", args, &out); err != nil {
		return nil, err
	}
	return out, nil
}
{{end}}
`

func main() {
	in := flag.String("in", "pdocs/steam_api_list.json", "path to GetSupportedAPIList JSON")
	out := flag.String("out", "internal/steam/generated.go", "output Go file")
	pkg := flag.String("pkg", "steam", "Go package name")
	services := flag.String("services", "", "comma-separated allowlist of interface names; empty = all")
	flag.Parse()

	raw, err := os.ReadFile(*in)
	if err != nil {
		log.Fatalf("gen-steam: read %s: %v", *in, err)
	}
	var list apiList
	if err := json.Unmarshal(raw, &list); err != nil {
		log.Fatalf("gen-steam: parse %s: %v", *in, err)
	}

	if allow := parseAllowlist(*services); allow != nil {
		filtered := list.APIList.Interfaces[:0]
		for _, svc := range list.APIList.Interfaces {
			if allow[svc.Name] {
				filtered = append(filtered, svc)
				delete(allow, svc.Name)
			}
		}
		list.APIList.Interfaces = filtered
		if len(allow) > 0 {
			missing := make([]string, 0, len(allow))
			for name := range allow {
				missing = append(missing, name)
			}
			sort.Strings(missing)
			log.Fatalf("gen-steam: -services entries not found in metadata: %s", strings.Join(missing, ", "))
		}
	}

	sort.Slice(list.APIList.Interfaces, func(i, j int) bool {
		return list.APIList.Interfaces[i].Name < list.APIList.Interfaces[j].Name
	})
	for k := range list.APIList.Interfaces {
		ms := list.APIList.Interfaces[k].Methods
		sort.Slice(ms, func(i, j int) bool {
			if ms[i].Name != ms[j].Name {
				return ms[i].Name < ms[j].Name
			}
			return ms[i].Version < ms[j].Version
		})
	}

	var methods []methodView
	seen := map[string]bool{}
	for _, svc := range list.APIList.Interfaces {
		for _, m := range svc.Methods {
			mv := buildMethodView(svc, m)
			if seen[mv.GoName] {
				mv.GoName = fmt.Sprintf("%sV%d", mv.GoName, m.Version)
				mv.StructName = mv.GoName + "Request"
			}
			seen[mv.GoName] = true
			methods = append(methods, mv)
		}
	}

	t := template.Must(template.New("file").Parse(fileTmpl))
	var buf bytes.Buffer
	if err := t.Execute(&buf, map[string]any{"Pkg": *pkg, "Methods": methods}); err != nil {
		log.Fatalf("gen-steam: render: %v", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		_ = os.WriteFile(*out+".broken", buf.Bytes(), 0o644)
		log.Fatalf("gen-steam: gofmt failed (unformatted dump at %s.broken): %v", *out, err)
	}

	if err := os.WriteFile(*out, formatted, 0o644); err != nil {
		log.Fatalf("gen-steam: write %s: %v", *out, err)
	}
	fmt.Printf("gen-steam: wrote %d methods to %s (%d bytes)\n", len(methods), *out, len(formatted))
}
