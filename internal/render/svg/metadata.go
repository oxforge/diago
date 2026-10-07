package svg

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oxforge/diago/internal/model"
)

// LayoutMetadataID is the id of the <metadata> element carrying the
// anchoring carrier (C18) in every flow and class SVG.
const LayoutMetadataID = "diago-layout"

// layoutMetadataJSON serializes the carrier for embedding; a nil carrier
// yields "" (no element).
func layoutMetadataJSON(h *model.LayoutHints) string {
	if h == nil {
		return ""
	}
	b, err := json.Marshal(h)
	if err != nil {
		return ""
	}
	return string(b)
}

// ExtractLayoutHints recovers the carrier from an SVG produced by Render.
// It errors when the element is absent, malformed, or of another version.
func ExtractLayoutHints(svgText string) (*model.LayoutHints, error) {
	open := `<metadata id="` + LayoutMetadataID + `">`
	start := strings.Index(svgText, open)
	if start < 0 {
		return nil, fmt.Errorf("svg: no <metadata id=%q> element (not a native diago flow render?)", LayoutMetadataID)
	}
	rest := svgText[start+len(open):]
	end := strings.Index(rest, "</metadata>")
	if end < 0 {
		return nil, fmt.Errorf("svg: unterminated <metadata id=%q>", LayoutMetadataID)
	}
	raw := unescapeXML(rest[:end])
	var h model.LayoutHints
	if err := json.Unmarshal([]byte(raw), &h); err != nil {
		return nil, fmt.Errorf("svg: layout metadata: %w", err)
	}
	if err := h.Validate(); err != nil {
		return nil, fmt.Errorf("svg: %w", err)
	}
	if h.Reversed == nil {
		h.Reversed = []string{}
	}
	return &h, nil
}

func unescapeXML(s string) string {
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	return strings.ReplaceAll(s, "&amp;", "&")
}
