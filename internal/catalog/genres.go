package catalog

import "strings"

// Genre identifiers stay language-independent across Steam, IGDB, saved catalogs
// and recommendation profiles. Only equivalent genres are folded: Steam Action
// does not imply Shooter. Commercial/content/software tags are not game genres.
var genreAliases = map[string]string{
	"accounting":                 "",
	"action":                     "Action",
	"adventure":                  "Adventure",
	"animation & modeling":       "",
	"arcade":                     "Arcade",
	"audio production":           "",
	"card & board game":          "Card & Board Game",
	"casual":                     "Casual",
	"early access":               "",
	"education":                  "",
	"fighting":                   "Fighting",
	"free to play":               "",
	"game development":           "",
	"hack and slash/beat 'em up": "Hack and slash/Beat 'em up",
	"indie":                      "Indie",
	"massively multiplayer":      "Massively Multiplayer",
	"moba":                       "MOBA",
	"music":                      "Music",
	"nudity":                     "",
	"photo editing":              "",
	"pinball":                    "Pinball",
	"platform":                   "Platform",
	"platformer":                 "Platform",
	"point-and-click":            "Point-and-click",
	"puzzle":                     "Puzzle",
	"quiz/trivia":                "Quiz/Trivia",
	"racing":                     "Racing",
	"real time strategy (rts)":   "Real Time Strategy (RTS)",
	"role-playing (rpg)":         "Role-playing (RPG)",
	"rpg":                        "Role-playing (RPG)",
	"sexual content":             "",
	"shooter":                    "Shooter",
	"simulation":                 "Simulator",
	"simulator":                  "Simulator",
	"software training":          "",
	"sport":                      "Sport",
	"sports":                     "Sport",
	"strategy":                   "Strategy",
	"tactical":                   "Tactical",
	"turn-based strategy (tbs)":  "Turn-based strategy (TBS)",
	"utilities":                  "",
	"video production":           "",
	"violent":                    "",
	"visual novel":               "Visual Novel",
	"анимация и моделирование": "",
	"бесплатно":                "",
	"бухгалтерия":              "",
	"гонки":                    "Racing",
	"инди":                     "Indie",
	"казуальные игры":          "Casual",
	"массовая многопользовательская игра": "Massively Multiplayer",
	"многопользовательские игры":          "Massively Multiplayer",
	"обработка фото":                      "",
	"образование":                         "",
	"обучение работе с по":                "",
	"приключения":                         "Adventure",
	"приключенческие игры":                "Adventure",
	"работа с видео":                      "",
	"работа со звуком":                    "",
	"разработка игр":                      "",
	"ранний доступ":                       "",
	"ролевые":                             "Role-playing (RPG)",
	"ролевые игры":                        "Role-playing (RPG)",
	"симуляторы":                          "Simulator",
	"спорт":                               "Sport",
	"стратегии":                           "Strategy",
	"утилиты":                             "",
	"шутеры":                              "Shooter",
	"экшен":                               "Action",
	"экшены":                              "Action",
}

func canonicalGenre(value string) string {
	value = strings.TrimSpace(value)
	if canonical, ok := genreAliases[strings.ToLower(value)]; ok {
		return canonical
	}
	return value
}

func canonicalGenres(values []string) []string {
	if values == nil {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = canonicalGenre(value)
		key := strings.ToLower(value)
		if value != "" && !seen[key] {
			result = append(result, value)
			seen[key] = true
		}
	}
	return result
}
