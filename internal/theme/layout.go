package theme

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"typhon/internal/uierr"
)

const maxLayoutIDBytes = 32

var ErrThemeLayout = uierr.New("theme.invalid_layout", "недопустимая раскладка темы")

type Layout struct {
	Sidebar  *SidebarLayout  `json:"sidebar,omitempty"`
	Topbar   *TopbarLayout   `json:"topbar,omitempty"`
	GamePage *GamePageLayout `json:"gamePage,omitempty"`
}

type SidebarLayout struct {
	Side   string   `json:"side,omitempty"`
	Order  []string `json:"order,omitempty"`
	Hidden []string `json:"hidden,omitempty"`
}

type TopbarLayout struct {
	Search string   `json:"search,omitempty"`
	Order  []string `json:"order,omitempty"`
	Hidden []string `json:"hidden,omitempty"`
}

type GamePageLayout struct {
	Main   []string `json:"main,omitempty"`
	Side   []string `json:"side,omitempty"`
	Hidden []string `json:"hidden,omitempty"`
}

type layoutSpec struct {
	Separator     string `json:"separator"`
	MaxSeparators int    `json:"maxSeparators"`
	Sidebar       struct {
		Sides        []string `json:"sides"`
		Items        []string `json:"items"`
		DefaultOrder []string `json:"defaultOrder"`
		Hideable     []string `json:"hideable"`
	} `json:"sidebar"`
	Topbar struct {
		SearchPositions []string `json:"searchPositions"`
		Actions         []string `json:"actions"`
	} `json:"topbar"`
	GamePage struct {
		Main []string `json:"main"`
		Side []string `json:"side"`
	} `json:"gamePage"`
	Parts       []string `json:"parts"`
	PartsWithID []string `json:"partsWithId"`
}

//go:embed layout.json
var layoutJSON []byte

var layoutIDs, layoutSpecErr = parseLayoutSpec(layoutJSON)

func parseLayoutSpec(data []byte) (layoutSpec, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var spec layoutSpec
	if err := dec.Decode(&spec); err != nil {
		return layoutSpec{}, fmt.Errorf("parse layout.json: %w", err)
	}
	if spec.Separator == "" || spec.MaxSeparators <= 0 {
		return layoutSpec{}, errors.New("layout.json: separator and maxSeparators are required")
	}
	lists := []struct {
		name string
		ids  []string
	}{
		{"sidebar.sides", spec.Sidebar.Sides},
		{"sidebar.items", spec.Sidebar.Items},
		{"sidebar.hideable", spec.Sidebar.Hideable},
		{"topbar.searchPositions", spec.Topbar.SearchPositions},
		{"topbar.actions", spec.Topbar.Actions},
		{"gamePage.main+side", append(slices.Clone(spec.GamePage.Main), spec.GamePage.Side...)},
		{"parts", spec.Parts},
		{"partsWithId", spec.PartsWithID},
	}
	for _, list := range lists {
		if len(list.ids) == 0 {
			return layoutSpec{}, fmt.Errorf("layout.json: %s is empty", list.name)
		}
		for i, id := range list.ids {
			if id == "" || id == spec.Separator || len(id) > maxLayoutIDBytes || slices.Contains(list.ids[:i], id) {
				return layoutSpec{}, fmt.Errorf("layout.json: %s: bad or repeated id %q", list.name, id)
			}
		}
	}
	if err := checkSidebarOrder(spec.Sidebar.DefaultOrder, spec); err != nil {
		return layoutSpec{}, fmt.Errorf("layout.json: sidebar.defaultOrder: %w", err)
	}
	return spec, nil
}

// normalizeLayout drops empty blocks so a theme without any layout choice
// stores and exports no layout object at all.
func normalizeLayout(l *Layout) *Layout {
	if l == nil {
		return nil
	}
	out := &Layout{}
	if sb := l.Sidebar; sb != nil && (sb.Side != "" || len(sb.Order) > 0 || len(sb.Hidden) > 0) {
		out.Sidebar = &SidebarLayout{Side: sb.Side, Order: nonEmpty(sb.Order), Hidden: nonEmpty(sb.Hidden)}
	}
	if tb := l.Topbar; tb != nil && (tb.Search != "" || len(tb.Order) > 0 || len(tb.Hidden) > 0) {
		out.Topbar = &TopbarLayout{Search: tb.Search, Order: nonEmpty(tb.Order), Hidden: nonEmpty(tb.Hidden)}
	}
	if gp := l.GamePage; gp != nil && (len(gp.Main) > 0 || len(gp.Side) > 0 || len(gp.Hidden) > 0) {
		out.GamePage = &GamePageLayout{Main: nonEmpty(gp.Main), Side: nonEmpty(gp.Side), Hidden: nonEmpty(gp.Hidden)}
	}
	if out.Sidebar == nil && out.Topbar == nil && out.GamePage == nil {
		return nil
	}
	return out
}

func nonEmpty(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	return slices.Clone(list)
}

func validateLayout(l *Layout) (*Layout, error) {
	l = normalizeLayout(l)
	if l == nil {
		return nil, nil
	}
	if layoutSpecErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrThemeLayout, layoutSpecErr)
	}
	spec := layoutIDs
	if sb := l.Sidebar; sb != nil {
		if err := checkChoice("sidebar.side", sb.Side, spec.Sidebar.Sides); err != nil {
			return nil, err
		}
		if err := checkSidebarOrder(sb.Order, spec); err != nil {
			return nil, err
		}
		if err := checkIDs("sidebar.hidden", sb.Hidden, spec.Sidebar.Hideable, nil); err != nil {
			return nil, err
		}
	}
	if tb := l.Topbar; tb != nil {
		if err := checkChoice("topbar.search", tb.Search, spec.Topbar.SearchPositions); err != nil {
			return nil, err
		}
		if err := checkIDs("topbar.order", tb.Order, spec.Topbar.Actions, nil); err != nil {
			return nil, err
		}
		if err := checkIDs("topbar.hidden", tb.Hidden, spec.Topbar.Actions, nil); err != nil {
			return nil, err
		}
	}
	if gp := l.GamePage; gp != nil {
		sections := append(slices.Clone(spec.GamePage.Main), spec.GamePage.Side...)
		if err := checkIDs("gamePage.main", gp.Main, sections, nil); err != nil {
			return nil, err
		}
		if err := checkIDs("gamePage.side", gp.Side, sections, gp.Main); err != nil {
			return nil, err
		}
		if err := checkIDs("gamePage.hidden", gp.Hidden, sections, nil); err != nil {
			return nil, err
		}
	}
	return l, nil
}

func checkLength(field, value string) error {
	if len(value) > maxLayoutIDBytes {
		return fmt.Errorf("%s: value of %d bytes is longer than %d: %w", field, len(value), maxLayoutIDBytes, ErrThemeLayout)
	}
	return nil
}

func checkChoice(field, value string, allowed []string) error {
	if value == "" {
		return nil
	}
	if err := checkLength(field, value); err != nil {
		return err
	}
	if !slices.Contains(allowed, value) {
		return fmt.Errorf("%s: unknown value %q: %w", field, value, ErrThemeLayout)
	}
	return nil
}

// checkIDs accepts ids from allowed, each at most once across list and taken
// (the ids another list of the same page already placed).
func checkIDs(field string, list, allowed, taken []string) error {
	if len(list) > len(allowed) {
		return fmt.Errorf("%s: %d entries, at most %d: %w", field, len(list), len(allowed), ErrThemeLayout)
	}
	for i, id := range list {
		if err := checkLength(field, id); err != nil {
			return err
		}
		if !slices.Contains(allowed, id) {
			return fmt.Errorf("%s: unknown id %q: %w", field, id, ErrThemeLayout)
		}
		if slices.Contains(list[:i], id) || slices.Contains(taken, id) {
			return fmt.Errorf("%s: id %q appears twice: %w", field, id, ErrThemeLayout)
		}
	}
	return nil
}

func checkSidebarOrder(order []string, spec layoutSpec) error {
	limit := len(spec.Sidebar.Items) + spec.MaxSeparators
	if len(order) > limit {
		return fmt.Errorf("sidebar.order: %d entries, at most %d: %w", len(order), limit, ErrThemeLayout)
	}
	separators := 0
	var items []string
	for _, id := range order {
		if id == spec.Separator {
			separators++
			continue
		}
		items = append(items, id)
	}
	if separators > spec.MaxSeparators {
		return fmt.Errorf("sidebar.order: %d separators, at most %d: %w", separators, spec.MaxSeparators, ErrThemeLayout)
	}
	return checkIDs("sidebar.order", items, spec.Sidebar.Items, nil)
}
