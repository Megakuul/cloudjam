// schemagen generates typesafe structures for service providers specific format.
// The output schemas are required for plugin developers.
// TODO: this package is very sloppy and only does cloud control right now.
// -> should be rewritten with proper cobra structure, maybe use a codegenerator like jen and support different providers.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

var (
	source   = flag.String("url", "https://schema.cloudformation.us-east-1.amazonaws.com/CloudformationSchema.zip", "schema bundle")
	output   = flag.String("out", ".", "output directory")
	services = flag.String("services", "", "comma separated services to generate (default: all)")
)

// schema is the part of a resource provider schema we generate from.
type schema struct {
	TypeName    string          `json:"typeName"`
	Properties  map[string]node `json:"properties"`
	Definitions map[string]node `json:"definitions"`
}

// node is one json schema node.
type node struct {
	Type                 json.RawMessage   `json:"type"`
	Ref                  string            `json:"$ref"`
	Enum                 []json.RawMessage `json:"enum"`
	Items                *node             `json:"items"`
	Properties           map[string]node   `json:"properties"`
	PatternProperties    map[string]node   `json:"patternProperties"`
	AdditionalProperties json.RawMessage   `json:"additionalProperties"`
}

// kind returns the json schema type, or "" when the node is a union, a
// combinator or untyped — all of which we fall back to raw json for.
func (n node) kind() string {
	var single string
	if json.Unmarshal(n.Type, &single) != nil {
		return ""
	}
	return single
}

func main() {
	flag.Parse()

	wanted := []string{}
	if *services != "" {
		wanted = strings.Split(*services, ",")
	}

	byService := map[string][]schema{}
	for _, s := range download(*source) {
		parts := strings.Split(s.TypeName, "::")
		if len(parts) != 3 || parts[0] != "AWS" {
			continue
		}
		service := strings.ToLower(parts[1])
		if len(wanted) > 0 && !slices.Contains(wanted, service) {
			continue
		}
		byService[service] = append(byService[service], s)
	}

	for service, resources := range byService {
		sort.Slice(resources, func(i, j int) bool { return resources[i].TypeName < resources[j].TypeName })

		g := &generator{service: service, names: map[string]string{}, taken: map[string]bool{}}
		for _, resource := range resources {
			g.resource(resource)
		}

		directory := filepath.Join(*output, service)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			log.Fatal(err)
		}
		code, err := format.Source(g.render())
		if err != nil {
			log.Fatalf("%s: %v", service, err)
		}
		if err := os.WriteFile(filepath.Join(directory, service+".go"), code, 0o644); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("generated %d services\n", len(byService))
}

func download(url string) []schema {
	response, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		log.Fatal(err)
	}

	schemas := []schema{}
	for _, entry := range archive.File {
		if !strings.HasSuffix(entry.Name, ".json") {
			continue
		}
		file, err := entry.Open()
		if err != nil {
			log.Fatal(err)
		}
		var s schema
		if err := json.NewDecoder(file).Decode(&s); err != nil {
			log.Printf("skipping %s: %v", entry.Name, err)
		} else {
			schemas = append(schemas, s)
		}
		file.Close()
	}
	return schemas
}

// object is one generated struct.
type object struct {
	name     string
	typeName string // set for resources, empty for property structs
	fields   []field
}

type field struct {
	name    string
	goType  string
	jsonTag string
}

// enum is a string property with a fixed set of values.
type enum struct {
	name      string
	constants []constant
}

type constant struct {
	name  string
	value string
}

type generator struct {
	service string
	objects []object
	enums   []enum

	// names maps a schema definition to the go type it became, taken records
	// every go type name already handed out in this package.
	names map[string]string
	taken map[string]bool

	resourceName string // the resource currently being walked
	definitions  map[string]node
	raw          bool // whether json.RawMessage ended up in the output
}

func (g *generator) resource(s schema) {
	g.resourceName = s.TypeName[strings.LastIndex(s.TypeName, ":")+1:]
	g.definitions = s.Definitions
	g.object(g.name(g.resourceName, g.resourceName), s.Properties, s.TypeName)
}

// object emits a struct with one field per property.
func (g *generator) object(name string, properties map[string]node, typeName string) {
	o := object{name: name, typeName: typeName}
	for _, property := range sorted(properties) {
		o.fields = append(o.fields, field{
			name:    identifier(property),
			goType:  g.goType(properties[property], name+identifier(property)),
			jsonTag: property,
		})
	}
	g.objects = append(g.objects, o)
}

// goType returns the go type for a schema node, emitting structs for objects it
// meets on the way. inline is the name an anonymous object would get.
func (g *generator) goType(n node, inline string) string {
	if n.Ref != "" {
		return g.ref(path.Base(n.Ref))
	}
	switch n.kind() {
	case "string":
		if named := g.enum(inline, inline, n.Enum); named != "" {
			return named
		}
		return "*string"
	case "boolean":
		return "*bool"
	case "integer":
		return "*int"
	case "number":
		return "*float64"
	case "array":
		if n.Items == nil {
			return g.rawType()
		}
		return "[]" + strings.TrimPrefix(g.goType(*n.Items, inline+"Item"), "*")
	case "object":
		return g.objectType(n, inline)
	default:
		return g.rawType()
	}
}

func (g *generator) objectType(n node, inline string) string {
	if len(n.Properties) > 0 {
		name := g.name(inline, inline)
		g.object(name, n.Properties, "")
		return "*" + name
	}
	// A free form object: a map of whatever its values are.
	for _, pattern := range sorted(n.PatternProperties) {
		return "map[string]" + strings.TrimPrefix(g.goType(n.PatternProperties[pattern], inline+"Value"), "*")
	}
	var additional node
	if json.Unmarshal(n.AdditionalProperties, &additional) == nil && (additional.Ref != "" || len(additional.Type) > 0) {
		return "map[string]" + strings.TrimPrefix(g.goType(additional, inline+"Value"), "*")
	}
	return "map[string]any"
}

// ref resolves a #/definitions reference to a go type, emitting it on first use.
func (g *generator) ref(definition string) string {
	key := g.resourceName + "." + definition
	if name := g.names[key]; name != "" {
		return "*" + name
	}
	n, ok := g.definitions[definition]
	if !ok {
		return g.rawType()
	}
	if len(n.Properties) == 0 {
		// A definition that is just an enum keeps its own name, so every
		// reference to it lands on the same type.
		if named := g.enum(key, definition, n.Enum); named != "" {
			return named
		}
		// Not a struct — an alias for a scalar, an array or a map. Register the
		// key first so a self referencing definition cannot recurse forever.
		g.names[key] = ""
		defer delete(g.names, key)
		return g.goType(n, identifier(definition))
	}
	name := g.name(key, definition)
	g.object(name, n.Properties, "")
	return "*" + name
}

// enum emits a named string type with one constant per value and returns the
// field type for it. It returns "" when there is nothing enumerable, so the
// caller can fall back to a plain string.
func (g *generator) enum(key, candidate string, values []json.RawMessage) string {
	if name := g.names[key]; name != "" {
		return "*" + name
	}
	unique := []string{}
	seen := map[string]bool{}
	for _, raw := range values {
		var value string
		if json.Unmarshal(raw, &value) != nil || value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	if len(unique) == 0 {
		return ""
	}

	e := enum{name: g.name(key, candidate)}
	for _, value := range unique {
		e.constants = append(e.constants, constant{
			name:  g.name(e.name+"."+value, e.name+camel(value)),
			value: value,
		})
	}
	g.enums = append(g.enums, e)
	return "*" + e.name
}

// name hands out a unique go type name and remembers it for key.
func (g *generator) name(key, candidate string) string {
	base := identifier(candidate)
	name := base
	if g.taken[name] {
		name = g.resourceName + base
	}
	for suffix := 2; g.taken[name]; suffix++ {
		name = fmt.Sprintf("%s%s%d", g.resourceName, base, suffix)
	}
	g.taken[name] = true
	g.names[key] = name
	return name
}

func (g *generator) rawType() string {
	g.raw = true
	return "json.RawMessage"
}

func (g *generator) render() []byte {
	out := &bytes.Buffer{}
	fmt.Fprintf(out, "// Code generated by go generate; DO NOT EDIT.\n\n")
	fmt.Fprintf(out, "// Package %s holds the cloud control resources of the aws %s service.\n", g.service, g.service)
	fmt.Fprintf(out, "package %s\n\n", g.service)
	if g.raw {
		fmt.Fprintf(out, "import \"encoding/json\"\n\n")
	}
	for _, o := range g.objects {
		fmt.Fprintf(out, "type %s struct {\n", o.name)
		for _, f := range o.fields {
			fmt.Fprintf(out, "\t%s %s `json:\"%s,omitempty\"`\n", f.name, f.goType, f.jsonTag)
		}
		fmt.Fprintf(out, "}\n\n")
		if o.typeName != "" {
			fmt.Fprintf(out, "func (%s) CloudControlType() string { return %q }\n\n", o.name, o.typeName)
		}
	}
	for _, e := range g.enums {
		fmt.Fprintf(out, "type %s string\n\nconst (\n", e.name)
		for _, c := range e.constants {
			fmt.Fprintf(out, "\t%s %s = %q\n", c.name, e.name, c.value)
		}
		fmt.Fprintf(out, ")\n\n")
	}
	return out.Bytes()
}

// camel turns an enum value into an identifier fragment: "us-east-1" becomes
// "UsEast1", "AES256" stays "AES256".
func camel(value string) string {
	out := ""
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) {
		out += strings.ToUpper(part[:1]) + part[1:]
	}
	if out == "" || out[0] >= '0' && out[0] <= '9' {
		out = "X" + out
	}
	return out
}

// identifier turns a property name into an exported go identifier.
func identifier(name string) string {
	clean := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, name)
	if clean == "" {
		return "Field"
	}
	if clean[0] >= '0' && clean[0] <= '9' {
		clean = "X" + clean
	}
	return strings.ToUpper(clean[:1]) + clean[1:]
}

func sorted[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
