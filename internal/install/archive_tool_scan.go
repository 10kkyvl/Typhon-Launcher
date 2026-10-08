package install

import "strings"

// Хвост вывода в килобайт нужен только для текста ошибки: фраза, по которой
// опознаётся причина, может остаться за его пределами, поэтому весь вывод
// просматривается по ходу чтения, а на стыке кусков помнится конец предыдущего.
type markerScan struct {
	markers []string
	carry   string
	keep    int
	hits    map[string]bool
}

func newMarkerScan(lists ...[]string) *markerScan {
	s := &markerScan{hits: map[string]bool{}}
	for _, list := range lists {
		for _, marker := range list {
			s.markers = append(s.markers, marker)
			s.keep = max(s.keep, len(marker)-1)
		}
	}
	return s
}

func (s *markerScan) feed(text string) {
	if s == nil || len(s.markers) == 0 {
		return
	}
	window := s.carry + strings.ToLower(text)
	for _, marker := range s.markers {
		if !s.hits[marker] && strings.Contains(window, marker) {
			s.hits[marker] = true
		}
	}
	s.carry = window[max(0, len(window)-s.keep):]
}

func (s *markerScan) absorb(other *markerScan) {
	if s == nil || other == nil {
		return
	}
	for marker := range other.hits {
		s.hits[marker] = true
	}
}

func (s *markerScan) found(markers []string) bool {
	if s == nil {
		return false
	}
	for _, marker := range markers {
		if s.hits[marker] {
			return true
		}
	}
	return false
}
