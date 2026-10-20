package sysreq

import (
	"regexp"
	"strings"
)

// Tiers are a coarse performance class from 1 (office-grade) to 11 (current
// flagship). They only order parts of the same era; a model absent from the
// tables has tier 0, which the comparison reads as "cannot tell".

var (
	nvidiaRe = regexp.MustCompile(`(?i)\b(?:gtx|rtx|gt)\s*-?\s*(\d{3,4})\s*(ti)?\b`)
	radeonHD = regexp.MustCompile(`(?i)\bhd\s*-?\s*(\d{4})\b`)
	radeonR  = regexp.MustCompile(`(?i)\b(r[579])\s*-?\s*(\d{3})\s*(x)?\b`)
	radeonRX = regexp.MustCompile(`(?i)\brx\s*-?\s*(\d{3,4})\s*(xtx|xt|gre)?\b`)
	vegaRe   = regexp.MustCompile(`(?i)\bvega\s*(56|64)\b`)
	intelHD  = regexp.MustCompile(`(?i)\b(uhd|hd)\s*graphics\s*(\d{3,4})?\b`)
	arcRe    = regexp.MustCompile(`(?i)\barc\W*(?:tm\W*)?([ab]\d{3})\b`)
	irisRe   = regexp.MustCompile(`(?i)\biris\b`)
	radIGP   = regexp.MustCompile(`(?i)\bradeon\W*(?:tm\W*)?(?:vega\s*\d+\s*)?graphics\b`)
)

var nvidiaTiers = map[string]int{
	"450": 2, "460": 3, "465": 3, "470": 4, "480": 4,
	"550": 3, "550ti": 3, "560": 4, "560ti": 4, "570": 4, "580": 5, "590": 5,
	"630": 1, "640": 2, "650": 3, "650ti": 4, "660": 5, "660ti": 5, "670": 5, "680": 5, "690": 6,
	"710": 1, "720": 1, "730": 2, "740": 3, "750": 4, "750ti": 4, "760": 5, "770": 5, "780": 6, "780ti": 6,
	"940": 3, "950": 5, "960": 6, "970": 7, "980": 7, "980ti": 8,
	"1030": 3, "1050": 4, "1050ti": 5, "1060": 6, "1070": 7, "1070ti": 8, "1080": 8, "1080ti": 9,
	"1650": 6, "1660": 7, "1660ti": 7,
	"2060": 8, "2070": 8, "2080": 9, "2080ti": 10,
	"3050": 7, "3060": 8, "3060ti": 9, "3070": 9, "3070ti": 10, "3080": 10, "3080ti": 10, "3090": 10,
	"4050": 8, "4060": 9, "4060ti": 9, "4070": 10, "4070ti": 10, "4080": 11, "4090": 11,
	"5050": 9, "5060": 10, "5060ti": 10, "5070": 11, "5070ti": 11, "5080": 11, "5090": 11,
}

var radeonTiers = map[string]int{
	"hd5450": 1, "hd5670": 2, "hd5770": 3, "hd5850": 4, "hd5870": 4,
	"hd6450": 1, "hd6670": 2, "hd6770": 3, "hd6850": 3, "hd6870": 4, "hd6950": 4, "hd6970": 5,
	"hd7750": 3, "hd7770": 3, "hd7790": 4, "hd7850": 4, "hd7870": 5, "hd7950": 5, "hd7970": 6,
	"r5230": 1, "r7240": 2, "r7250": 3, "r7260": 4, "r7260x": 4, "r7265": 4,
	"r9270": 5, "r9270x": 5, "r9280": 5, "r9280x": 6, "r9285": 6, "r9290": 6, "r9290x": 7,
	"r9380": 5, "r9390": 7, "r9fury": 8,
	"rx460": 4, "rx470": 6, "rx480": 7, "rx550": 4, "rx560": 5, "rx570": 6, "rx580": 7, "rx590": 7,
	"rx5500xt": 6, "rx5600xt": 7, "rx5700": 8, "rx5700xt": 8,
	"rx6500xt": 6, "rx6600": 8, "rx6600xt": 8, "rx6650xt": 8, "rx6700xt": 9, "rx6800": 9, "rx6800xt": 10, "rx6900xt": 10,
	"rx7600": 8, "rx7700xt": 9, "rx7800xt": 10, "rx7900gre": 10, "rx7900xt": 10, "rx7900xtx": 11,
	"rx9060xt": 10, "rx9070": 11, "rx9070xt": 11,
	"vega56": 8, "vega64": 8,
}

var intelGPUTiers = map[string]int{
	"hd2000": 1, "hd2500": 1, "hd3000": 1, "hd4000": 1, "hd4400": 2, "hd4600": 2, "hd5000": 2, "hd5500": 2,
	"hd510": 2, "hd520": 2, "hd530": 2, "hd620": 2,
	"uhd600": 2, "uhd610": 2, "uhd620": 2, "uhd630": 2, "uhd730": 3, "uhd750": 3, "uhd770": 3,
	"a380": 5, "a580": 6, "a750": 8, "a770": 8, "b570": 8, "b580": 8,
}

func gpuTier(text string) int {
	if m := nvidiaRe.FindStringSubmatch(text); m != nil {
		return lookup(nvidiaTiers, m[1], strings.ToLower(m[2]))
	}
	if m := radeonRX.FindStringSubmatch(text); m != nil {
		return lookup(radeonTiers, "rx"+m[1], strings.ToLower(m[2]))
	}
	if m := radeonR.FindStringSubmatch(text); m != nil {
		return lookup(radeonTiers, strings.ToLower(m[1])+m[2], strings.ToLower(m[3]))
	}
	if m := radeonHD.FindStringSubmatch(text); m != nil {
		return lookup(radeonTiers, "hd"+m[1], "")
	}
	if m := vegaRe.FindStringSubmatch(text); m != nil {
		return radeonTiers["vega"+m[1]]
	}
	if m := arcRe.FindStringSubmatch(text); m != nil {
		return intelGPUTiers[strings.ToLower(m[1])]
	}
	if irisRe.MatchString(text) {
		return 3
	}
	if m := intelHD.FindStringSubmatch(text); m != nil {
		if m[2] == "" {
			if strings.EqualFold(m[1], "uhd") {
				return 2
			}
			return 1
		}
		return intelGPUTiers[strings.ToLower(m[1])+m[2]]
	}
	if radIGP.MatchString(text) {
		return 2
	}
	return 0
}

func lookup(table map[string]int, base, suffix string) int {
	if suffix != "" {
		if t, ok := table[base+suffix]; ok {
			return t
		}
	}
	return table[base]
}

var altSplit = regexp.MustCompile(`(?i)\s+or\s+|\s+или\s+|\s*/\s*|\s*\|\s*|;`)

var fillerAlt = regexp.MustCompile(`(?i)\b(equivalent|better|higher|newer|faster|compatible)\b|аналог|лучше|выше|новее|совместим`)

// minTier returns the least demanding alternative among those it can rate:
// the user meets an "A or B" requirement by meeting either part. partial is
// true when some alternative is a real part name that is not in the tables,
// so a machine below the rated tier may still meet the unrated alternative.
// Filler such as "or equivalent" is not an alternative.
func minTier(text string, rate func(string) int) (tier int, partial bool) {
	for _, alt := range altSplit.Split(text, -1) {
		if strings.TrimSpace(alt) == "" {
			continue
		}
		t := rate(alt)
		switch {
		case t > 0 && (tier == 0 || t < tier):
			tier = t
		case t == 0 && !fillerAlt.MatchString(alt):
			partial = true
		}
	}
	return tier, partial && tier > 0
}
