package octanox

import (
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
)

type tsCodeBuilder struct {
	sb  strings.Builder
	ind int
}

func (b *tsCodeBuilder) write(s string) {
	b.sb.WriteString(s)
}

func (b *tsCodeBuilder) writeLine(s string) {
	b.write(strings.Repeat(" ", b.ind))
	b.write(s)
	b.write("\n")
}

func (b *tsCodeBuilder) writeLines(strs ...string) {
	for _, s := range strs {
		b.writeLine(s)
	}
}

func (b *tsCodeBuilder) indent() {
	b.ind += 2
}

func (b *tsCodeBuilder) unindent() {
	b.ind -= 2
}

func (i *Instance) generateTypeScriptClientCode(path string, routes []route) {
	builder := tsCodeBuilder{
		ind: 0,
		sb:  strings.Builder{},
	}

	builder.writeLines(
		"// This file is generated by Octanox. Do not edit this file manually.",
		"//",
		"// This file contains the TypeScript client code for the Octanox server.",
		"",
		"let baseUrl = window.location.origin",
		"let unauthorizedHandler: () => void",
		"",
		"export function setBaseUrl(url: string) {",
		"  baseUrl = url",
		"}",
		"",
		"export function setUnauthorizedHandler(handler: () => void) {",
		"  unauthorizedHandler = handler",
		"}",
		"",
		"function getBaseConfig(): RequestInit {",
		"  return {",
	)

	if i.Authenticator != nil {
		authMethod := i.Authenticator.Method()
		if authMethod == AuthenticationMethodBearer {
			builder.writeLines(
				"    headers: {",
				" 		 'Authorization': `Bearer ${localStorage.getItem('token')}`",
				"    },",
			)
		} else if authMethod == AuthenticationMethodBasic {
			builder.writeLines(
				"    headers: {",
				"      'Authorization': `Basic ${btoa(`${localStorage.getItem('username')}:${localStorage.getItem('password')}`)}`",
				"    },",
			)
		} else if authMethod == AuthenticationMethodApiKey {
			builder.writeLines(
				"    headers: {",
				"      'X-API-Key': localStorage.getItem('apiKey')",
				"    },",
			)
		}
	}

	builder.writeLines(
		"  }",
		"}",
		"",
		"async function fetchJson<T>(url: string, init?: RequestInit): Promise<T> {",
		"  let response = await fetch(baseUrl + url, {...getBaseConfig(), ...init})",
		"  if (response.status === 401) {",
		"    unauthorizedHandler()",
		"  }",
		"  if (!response.ok) {",
		"    throw new Error(`Failed to fetch ${url}: ${response.statusText}`)",
		"  }",
		"  return await response.json()",
		"}",
		"",
	)

	if i.Authenticator != nil {
		builder.writeLines(
			"export async function login(username: string, password: string): Promise<string> {",
			"  let formData = new FormData()",
			"  formData.append('username', username)",
			"  formData.append('password', password)",
			"  let response = await fetch(baseUrl + '"+i.authLoginBasePath+"', {",
			"    method: 'POST',",
			"    body: formData",
			"  })",
			"  if (!response.ok) {",
			"    throw new Error(`Failed to login: ${response.statusText}`)",
			"  }",
			"  let {token} = await response.json()",
			"  localStorage.setItem('token', token)",
			"  return token",
			"}",
			"",
		)
	}

	// Generate interfaces for the structs in the request body
	for _, route := range routes {
		if route.requestType != nil && route.responseType.Name() != "" {
			builder.generateBodyInterface(route.requestType)
			builder.writeLine("")
		}

		if route.responseType != nil && route.responseType.Name() != "" {
			builder.generateStructInterface(route.responseType)
			builder.writeLine("")
		}
	}

	// Generate functions for each route
	for _, route := range routes {
		builder.generateRouteFunction(route)
		builder.writeLine("")
	}

	builder.writeLines("// end of generated code")

	err := os.WriteFile(path, []byte(builder.sb.String()), 0644)
	if err != nil {
		panic(err)
	}
}

func (tb *tsCodeBuilder) generateRouteFunction(route route) {
	tb.write("export async function " + tb.generateFunctionName(route) + "(")
	if route.requestType != nil {
		tb.generateFunctionParameters(route.requestType)
	}

	tb.write("): Promise<")
	tb.typeFromGo(route.responseType)
	tb.writeLine("> {")

	tb.indent()
	tb.writeLine("let url = `" + route.path + "`")

	for i := 0; i < route.requestType.NumField(); i++ {
		field := route.requestType.Field(i)
		if pathParam := field.Tag.Get("path"); pathParam != "" {
			tb.writeLine("url = url.replace(`:" + pathParam + "`, encodeURIComponent(" + field.Name + ".toString()))")
		}
	}

	tb.writeLine("const config: RequestInit = {")
	tb.indent()
	tb.writeLine("method: '" + strings.ToUpper(route.method) + "',")

	if route.requestType != nil {
		if route.method != http.MethodGet && route.requestType.NumField() > 0 {
			tb.writeLine("body: JSON.stringify(" + tb.getBodyParamName(route.requestType) + "),")
			tb.writeLine("headers: { 'Content-Type': 'application/json' },")
		}
	}

	tb.unindent()
	tb.writeLine("};")

	if route.requestType != nil {
		for i := 0; i < route.requestType.NumField(); i++ {
			field := route.requestType.Field(i)
			if queryParam := field.Tag.Get("query"); queryParam != "" {
				tb.writeLine("url += `" + tb.getQueryParamString(queryParam, field.Name) + "`")
			}
		}
	}

	tb.write("  return fetchJson<")
	tb.typeFromGo(route.responseType)
	tb.unindent()
	tb.writeLine(">(url, config);")
	tb.writeLine("}")
}

func (tb *tsCodeBuilder) generateFunctionName(route route) string {
	path := strings.Replace(route.path, os.Getenv("NOX__GEN_OMIT_URL"), "", 1)
	path = strings.ReplaceAll(path, "/", "_")
	path = strings.ReplaceAll(path, ":", "")
	return strings.ToLower(route.method) + path
}

func (tb *tsCodeBuilder) generateFunctionParameters(t reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Anonymous {
			continue
		}
		tb.write(field.Name + ": ")
		tb.typeFromGo(field.Type)

		if i < t.NumField()-1 {
			tb.write(", ")
		}
	}
}

func (tb *tsCodeBuilder) getBodyParamName(t reflect.Type) string {
	for i := 0; i < t.NumField(); i++ {
		if bodyTag := t.Field(i).Tag.Get("body"); bodyTag != "" {
			return t.Field(i).Name
		}
	}
	return ""
}

func (tb *tsCodeBuilder) getQueryParamString(queryParam, fieldName string) string {
	return fmt.Sprintf("?%s=${encodeURIComponent(%s.toString())}", queryParam, fieldName)
}

func (tb *tsCodeBuilder) generateStructInterface(t reflect.Type) {
	if t.Kind() != reflect.Struct {
		return
	}

	tb.writeLine("export interface " + t.Name() + " {")
	tb.generateStructBody(t, false)
	tb.writeLine("}")
}

func (tb *tsCodeBuilder) generateBodyInterface(t reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if bodyTag := field.Tag.Get("body"); bodyTag != "" {
			tb.generateStructInterface(field.Type)
		}
	}
}

func (tb *tsCodeBuilder) generateStructBody(t reflect.Type, inline bool) {
	if t.Kind() != reflect.Struct {
		return
	}

	if !inline {
		tb.indent()
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip embedded fields
		if field.Anonymous {
			continue
		}

		jsonTag := field.Tag.Get("json")
		jsonName := field.Name
		omitempty := false
		if jsonTag != "" {
			if jsonTag == "-" {
				continue
			}

			jsonName = jsonTag
			if strings.Contains(jsonTag, ",omitempty") {
				omitempty = true
			}
		}

		tb.write(strings.Repeat(" ", tb.ind))
		tb.write(jsonName + ": ")
		tb.typeFromGo(field.Type)
		if omitempty {
			tb.write(" | undefined")
		}

		tb.write(";")
		tb.writeLine("")
	}

	if !inline {
		tb.unindent()
	}
}

func (tb *tsCodeBuilder) typeFromGo(t reflect.Type) {
	switch t.Kind() {
	case reflect.Ptr:
		tb.typeFromGo(t.Elem())
		tb.write(" | null")
		return
	case reflect.String:
		tb.write("string")
		return
	case reflect.Bool:
		tb.write("boolean")
		return
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		tb.write("number")
		return
	case reflect.Struct:
		// if it's an anonymous struct, generate an inline interface
		if t.Name() == "" {
			tb.write("{")
			tb.generateStructBody(t, true)
			tb.write("}")
			return
		}

		tb.write(t.Name())
	case reflect.Slice:
		tb.write("Array<")
		tb.typeFromGo(t.Elem())
		tb.write(">")
		return
	default:
		tb.write("any")
		return
	}
}
