package api

import (
	"net/http"
)

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, openAPIDoc())
}

// openAPIDoc is a compact but valid OpenAPI 3.1 description of the API, so
// an autonomous agent can discover the surface from one request.
func openAPIDoc() map[string]any {
	obj := func(props map[string]any, required ...string) map[string]any {
		m := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			m["required"] = required
		}
		return m
	}
	ref := func(name string) map[string]any {
		return map[string]any{"$ref": "#/components/schemas/" + name}
	}
	arr := func(items any) map[string]any {
		return map[string]any{"type": "array", "items": items}
	}
	str := map[string]any{"type": "string"}
	num := map[string]any{"type": "number"}
	integer := map[string]any{"type": "integer"}
	boolean := map[string]any{"type": "boolean"}

	jsonBody := func(schema any) map[string]any {
		return map[string]any{
			"required": true,
			"content":  map[string]any{"application/json": map[string]any{"schema": schema}},
		}
	}
	ok := func(desc string, schema any) map[string]any {
		return map[string]any{
			"description": desc,
			"content":     map[string]any{"application/json": map[string]any{"schema": schema}},
		}
	}
	errResp := map[string]any{"description": "error", "content": map[string]any{
		"application/json": map[string]any{"schema": ref("Error")},
	}}
	op := func(summary string, extra map[string]any) map[string]any {
		m := map[string]any{"summary": summary, "responses": map[string]any{"200": ok("ok", nil), "400": errResp, "404": errResp, "500": errResp}}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "typeshit handwriting API",
			"version":     Version,
			"description": "Turns text into rendered handwritten pages using a captured handwriting profile.",
		},
		"servers": []any{map[string]any{"url": "/"}},
		"paths": map[string]any{
			"/api/v1/health": map[string]any{"get": op("service status", nil)},
			"/api/v1/fonts":  map[string]any{"get": op("bundled fallback fonts", nil)},
			"/api/v1/profiles": map[string]any{
				"get": op("list handwriting profiles", nil),
				"post": op("build a profile from a bundled font", map[string]any{
					"requestBody": jsonBody(obj(map[string]any{
						"name":     str,
						"fontId":   str,
						"variants": integer,
						"lang":     str,
					})),
				}),
			},
			"/api/v1/profiles/{id}": map[string]any{
				"get":    op("full profile with glyph sprites", nil),
				"delete": op("delete a profile", nil),
			},
			"/api/v1/profiles/{id}/preview": map[string]any{
				"get": op("render a test line with the profile", map[string]any{
					"parameters": []any{map[string]any{"name": "text", "in": "query", "schema": str}},
				}),
			},
			"/api/v1/template": map[string]any{
				"get": op("printable sample sheets for capturing a handwriting", map[string]any{
					"parameters": []any{
						map[string]any{"name": "lang", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"ru", "ru-short", "latin"}}},
						map[string]any{"name": "reps", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": 3}},
					},
				}),
			},
			"/api/v1/capture": map[string]any{
				"post": op("analyse a photographed sample sheet", map[string]any{
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{"multipart/form-data": map[string]any{"schema": obj(map[string]any{
							"file": map[string]any{"type": "string", "format": "binary"},
							"spec": map[string]any{"type": "string", "description": "SheetSpec JSON"},
						}, "file")}},
					},
				}),
			},
			"/api/v1/capture/{id}":        map[string]any{"get": op("capture document with glyph cells", nil)},
			"/api/v1/capture/{id}/cells":  map[string]any{"post": op("replace the manual markup", nil)},
			"/api/v1/capture/{id}/commit": map[string]any{"post": op("cut sprites and create the profile", nil)},
			"/api/v1/generate": map[string]any{
				"post": op("render handwritten pages", map[string]any{
					"requestBody": jsonBody(ref("GenerateRequest")),
				}),
			},
			"/api/v1/jobs/{id}":               map[string]any{"get": op("asynchronous job status", nil)},
			"/api/v1/results/{id}/export.pdf": map[string]any{"get": op("document as PDF", nil)},
			"/api/v1/results/{id}/export.zip": map[string]any{"get": op("pages as ZIP", nil)},
			"/api/v1/results/{id}/{file}":     map[string]any{"get": op("single rendered page (PNG)", nil)},
			"/api/v1/openapi.json":            map[string]any{"get": op("this document", nil)},
		},
		"components": map[string]any{
			"schemas": map[string]any{
				"Error": obj(map[string]any{"error": str, "code": str}, "error", "code"),
				"GenerateRequest": obj(map[string]any{
					"text":      map[string]any{"type": "string", "maxLength": maxRunes},
					"profileId": str,
					"seed":      integer,
					"title":     str,
					"format":    map[string]any{"type": "string", "enum": []string{"png", "jpeg", "pdf"}},
					"page": obj(map[string]any{
						"columns": integer, "lineHeight": num, "indent": num,
						"align": map[string]any{"type": "string", "enum": []string{"left", "center", "justify"}},
						"width": integer, "height": integer,
						"marginTop": num, "marginSide": num, "marginBot": num,
						"skipLines": integer, "pageNumber": boolean,
					}),
					"paper": obj(map[string]any{
						"kind":       map[string]any{"type": "string", "enum": []string{"lined", "grid", "dot", "blank", "custom"}},
						"color":      str,
						"cellMm":     num,
						"marginMm":   num,
						"aging":      num,
						"marginSide": map[string]any{"type": "string", "enum": []string{"left", "right", "none"}},
					}),
					"pen": obj(map[string]any{
						"kind":       map[string]any{"type": "string", "enum": []string{"ballpoint", "gel", "pencil", "fountain"}},
						"color":      str,
						"width":      num,
						"inkDensity": num,
						"pressure":   num,
						"bleed":      num,
					}),
					"style": obj(map[string]any{
						"scale": num, "slant": num, "lineGap": num, "wordGap": num,
						"variability": num, "join": num,
					}),
					"effects": obj(map[string]any{
						"kind":  map[string]any{"type": "string", "enum": []string{"none", "scan", "phone", "fax"}},
						"noise": num, "blur": num, "jpeg": integer, "vignette": num,
						"shadow": num, "rotateDeg": num, "perspective": num,
						"contrast": num, "deskew": boolean, "resample": num,
					}),
					"flaws": obj(map[string]any{
						"lineWobble": num, "crossOut": num, "correction": num,
						"overwrite": num, "smudge": num, "blot": num, "stain": num,
						"marginNote": num, "skipWord": num, "underline": num,
						"crease": num, "pressureDot": num,
					}),
				}, "text", "profileId"),
				"CaptureCell": obj(map[string]any{
					"id": integer, "rect": arr(integer), "label": str,
					"row": integer, "col": integer, "score": num, "manual": boolean,
				}, "id", "rect"),
				"CaptureDoc": obj(map[string]any{
					"id": str, "image": str, "w": integer, "h": integer,
					"cells": arr(ref("CaptureCell")), "em": num, "slant": num,
					"notes": arr(str),
				}),
			},
		},
	}
}
