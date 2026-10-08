package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/oxforge/diago/internal/model"
	svgrender "github.com/oxforge/diago/internal/render/svg"
	"github.com/oxforge/diago/internal/schema"
	"github.com/oxforge/diago/internal/theme"
)

// ParsePrevious turns a -previous file into a carrier. It accepts, sniffed
// by content: an SVG diago rendered (its metadata is the exact anchor; one
// golayout rendered before the cutover to the layered engine anchors as a
// best effort), a carrier JSON, or a spec of the same type as the diagram
// being rendered (specType). A flow or class spec is laid out fresh
// as the render anchored on it lays out its own spec (S13): under the text
// profile when format is text ("text" or "txt"), else under the screen
// profile of the theme themeName ("" for the default), the render's
// -format and the theme it uses (RenderTheme of the render's spec and
// -theme, never the previous spec's field); so it is exact for one step,
// since layout is deterministic. An SVG or a carrier is what it is,
// whatever the format and the theme. A sequence render discards its
// previous (sequence diagrams have no layout freedom), so for specType
// "sequence" the file is only checked to be what -previous accepts, any
// SVG, a carrier or a valid sequence spec, and an empty carrier stands for
// it. Every failure of the file itself is a schema.ValidationErrors on
// field "previous".
func ParsePrevious(ctx context.Context, data []byte, specType, format, themeName string) (*model.LayoutHints, error) {
	trimmed := bytes.TrimSpace(data)
	switch {
	case bytes.HasPrefix(trimmed, []byte("<?xml")) || bytes.HasPrefix(trimmed, []byte("<svg")):
		if specType == "sequence" {
			return model.NewLayoutHints(), nil
		}
		h, err := svgrender.ExtractLayoutHints(string(trimmed))
		if err != nil {
			return nil, previousError(err.Error())
		}
		return h, nil
	case bytes.HasPrefix(trimmed, []byte("{")):
		var probe struct {
			Version *int            `json:"version"`
			Scopes  json.RawMessage `json:"scopes"`
			Type    string          `json:"type"`
		}
		if err := json.Unmarshal(trimmed, &probe); err != nil {
			return nil, previousError("not valid JSON: " + err.Error())
		}
		if probe.Version != nil || probe.Scopes != nil {
			var h model.LayoutHints
			if err := json.Unmarshal(trimmed, &h); err != nil {
				return nil, previousError("not a layout carrier: " + err.Error())
			}
			if err := h.Validate(); err != nil {
				return nil, previousError(err.Error())
			}
			if h.Reversed == nil {
				h.Reversed = []string{}
			}
			return &h, nil
		}
		if probe.Type != specType {
			return nil, previousError(fmt.Sprintf("a previous spec must be a %s diagram, got type %q", specType, probe.Type))
		}
		if specType == "sequence" {
			if _, err := schema.ParseSequence(trimmed); err != nil {
				return nil, previousError("previous spec: " + err.Error())
			}
			return model.NewLayoutHints(), nil
		}
		return previousFromSpec(ctx, trimmed, specType, format, themeName)
	default:
		return nil, previousError(fmt.Sprintf("not an SVG, a layout carrier, or a %s spec", specType))
	}
}

// previousFromSpec lays a spec of specType out fresh as a render in
// format with the theme themeName lays out its own spec, under the text
// profile or under the theme's screen profile (S13), and returns its
// carrier.
func previousFromSpec(ctx context.Context, spec []byte, specType, format, themeName string) (*model.LayoutHints, error) {
	parse := graphParser(schema.ParseFlow)
	if specType == "class" {
		parse = schema.ParseClass
	}
	g, err := parse(spec)
	if err != nil {
		return nil, previousError("previous spec: " + err.Error())
	}
	text := isTextFormat(format)
	var th theme.Theme
	if !text {
		if th, err = resolveTheme(themeName); err != nil {
			return nil, fmt.Errorf("previous: theme: %w", err)
		}
	}
	pg, err := layoutWith(ctx, *g, th, text, nil)
	if err != nil {
		return nil, fmt.Errorf("previous: layout: %w", err)
	}
	if pg.LayoutHints == nil {
		return nil, previousError("the layout emitted no carrier")
	}
	return pg.LayoutHints, nil
}

func previousError(msg string) error {
	return schema.ValidationErrors{{Field: "previous", Message: msg}}
}
