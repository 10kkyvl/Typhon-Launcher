// Package dialogtext supplies app-owned native dialog labels. The resolved UI
// language is passed with each request, including when settings follow the OS.
package dialogtext

type Labels struct {
	AllFiles, Executables                  string
	OpenTyphon, Quit                       string
	TorrentTitle, Torrents                 string
	FeedTitle, Feeds                       string
	TargetFolder                           string
	ThemeTitle, Themes, ExportTheme, Theme string
	AvatarTitle, AvatarImages              string
	CoverTitle, CoverImages                string
}

func For(language string) Labels {
	if language == "ru" {
		return Labels{
			OpenTyphon: "Открыть Typhon", Quit: "Выход",
			AllFiles: "Все файлы", Executables: "Исполняемые файлы (*.exe)",
			TorrentTitle: "Выберите торрент-файл", Torrents: "Торрент-файлы (*.torrent)",
			FeedTitle: "Выберите файл источника", Feeds: "Файл источника (*.json)",
			TargetFolder: "Выберите папку назначения",
			ThemeTitle:   "Выберите файл темы", Themes: "Файл темы (*.typhontheme, *.json)",
			ExportTheme: "Сохранить тему", Theme: "Файл темы (*.typhontheme)",
			AvatarTitle: "Выберите аватар", AvatarImages: "Изображения (*.png, *.jpg, *.jpeg, *.webp, *.gif)",
			CoverTitle: "Выберите обложку профиля", CoverImages: "Изображения (*.png, *.jpg, *.jpeg, *.webp)",
		}
	}
	return Labels{
		OpenTyphon: "Open Typhon", Quit: "Quit",
		AllFiles: "All files", Executables: "Executable files (*.exe)",
		TorrentTitle: "Choose a torrent file", Torrents: "Torrent files (*.torrent)",
		FeedTitle: "Choose a source file", Feeds: "Source file (*.json)",
		TargetFolder: "Choose the destination folder",
		ThemeTitle:   "Choose a theme file", Themes: "Theme file (*.typhontheme, *.json)",
		ExportTheme: "Save theme", Theme: "Theme file (*.typhontheme)",
		AvatarTitle: "Choose an avatar", AvatarImages: "Images (*.png, *.jpg, *.jpeg, *.webp, *.gif)",
		CoverTitle: "Choose a profile cover", CoverImages: "Images (*.png, *.jpg, *.jpeg, *.webp)",
	}
}
