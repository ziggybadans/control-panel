package mc

import "strings"

// MOTDSegment is one styled run of MOTD text, ready for the UI to render.
// Produced from either legacy §-codes or JSON chat components, so plugins
// like MiniMOTD that rewrite the advertised MOTD are shown faithfully.
type MOTDSegment struct {
	Text      string `json:"text"`
	Color     string `json:"color,omitempty"` // minecraft color name or #rrggbb
	Bold      bool   `json:"bold,omitempty"`
	Italic    bool   `json:"italic,omitempty"`
	Underline bool   `json:"underline,omitempty"`
	Strike    bool   `json:"strike,omitempty"`
}

type motdStyle struct {
	color                           string
	bold, italic, underline, strike bool
}

var legacyColors = map[rune]string{
	'0': "black", '1': "dark_blue", '2': "dark_green", '3': "dark_aqua",
	'4': "dark_red", '5': "dark_purple", '6': "gold", '7': "gray",
	'8': "dark_gray", '9': "blue", 'a': "green", 'b': "aqua",
	'c': "red", 'd': "light_purple", 'e': "yellow", 'f': "white",
}

// ParseLegacyMOTD converts a §-coded string into segments.
func ParseLegacyMOTD(s string) []MOTDSegment {
	var out []MOTDSegment
	appendLegacy(s, motdStyle{}, &out)
	return out
}

// appendLegacy walks text (which may contain §-codes) starting from base
// style. Vanilla semantics: a color code resets formatting; §r resets all.
func appendLegacy(text string, base motdStyle, out *[]MOTDSegment) {
	st := base
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			*out = append(*out, segment(cur.String(), st))
			cur.Reset()
		}
	}
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '§' || i+1 >= len(runes) {
			cur.WriteRune(runes[i])
			continue
		}
		code := runes[i+1]
		if code >= 'A' && code <= 'Z' {
			code += 'a' - 'A'
		}
		i++
		flush()
		if color, ok := legacyColors[code]; ok {
			st = motdStyle{color: color}
			continue
		}
		switch code {
		case 'l':
			st.bold = true
		case 'o':
			st.italic = true
		case 'n':
			st.underline = true
		case 'm':
			st.strike = true
		case 'r':
			st = base
		case 'k':
			// Obfuscated ("matrix") text: render plainly.
		default:
			// Unknown code: drop it, keep the text.
		}
	}
	flush()
}

func segment(text string, st motdStyle) MOTDSegment {
	return MOTDSegment{
		Text: text, Color: st.color,
		Bold: st.bold, Italic: st.italic, Underline: st.underline, Strike: st.strike,
	}
}

// motdFromDescription flattens a status-response "description" — a plain
// string, a chat component object, or an array of them.
func motdFromDescription(v any) []MOTDSegment {
	var out []MOTDSegment
	flattenComponent(v, motdStyle{}, &out)
	// Drop pure-empty segments so the UI doesn't render ghosts.
	kept := out[:0]
	for _, s := range out {
		if s.Text != "" {
			kept = append(kept, s)
		}
	}
	return kept
}

func flattenComponent(v any, st motdStyle, out *[]MOTDSegment) {
	switch t := v.(type) {
	case string:
		appendLegacy(t, st, out)
	case []any:
		for _, e := range t {
			flattenComponent(e, st, out)
		}
	case map[string]any:
		ns := st
		if c, ok := t["color"].(string); ok {
			ns.color = c
		}
		ns.bold = boolField(t, "bold", ns.bold)
		ns.italic = boolField(t, "italic", ns.italic)
		ns.underline = boolField(t, "underlined", ns.underline)
		ns.strike = boolField(t, "strikethrough", ns.strike)
		if txt, ok := t["text"].(string); ok {
			appendLegacy(txt, ns, out)
		}
		if extra, ok := t["extra"]; ok {
			flattenComponent(extra, ns, out)
		}
	}
}

// boolField reads a component flag that may arrive as bool or "true"/"false".
func boolField(m map[string]any, key string, def bool) bool {
	switch v := m[key].(type) {
	case bool:
		return v
	case string:
		if v == "true" {
			return true
		}
		if v == "false" {
			return false
		}
	}
	return def
}
