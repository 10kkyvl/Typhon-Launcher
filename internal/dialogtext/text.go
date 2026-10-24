// Package dialogtext supplies app-owned native dialog labels. The resolved UI
// language is passed with each request, including when settings follow the OS.
package dialogtext

type Labels struct {
	AllFiles, Executables                  string
	OpenTyphon, Quit                       string
	RecentGames, Playing, StopGame         string
	LibraryTask, DownloadsTask             string
	TorrentTitle, Torrents                 string
	FeedTitle, Feeds                       string
	TargetFolder                           string
	ThemeTitle, Themes, ExportTheme, Theme string
	SkillFolder                            string
	AvatarTitle, AvatarImages              string
	CoverTitle, CoverImages                string
	ChatImageTitle, ChatImages             string
	RollbackTitle                          string
	// RollbackAsk takes the version to go back to, RollbackFailed the reason.
	RollbackAsk, RollbackFailed string
}

func For(language string) Labels {
	if language == "ru" {
		return Labels{
			OpenTyphon: "Открыть Typhon", Quit: "Выход",
			RecentGames: "Недавние игры", Playing: "играет", StopGame: "Остановить",
			LibraryTask: "Библиотека", DownloadsTask: "Загрузки",
			AllFiles: "Все файлы", Executables: "Исполняемые файлы (*.exe)",
			TorrentTitle: "Выберите торрент-файл", Torrents: "Торрент-файлы (*.torrent)",
			FeedTitle: "Выберите файл источника", Feeds: "Файл источника (*.json)",
			TargetFolder: "Выберите папку назначения",
			ThemeTitle:   "Выберите файл темы", Themes: "Файл темы (*.typhontheme, *.json)",
			ExportTheme: "Сохранить тему", Theme: "Файл темы (*.typhontheme)",
			SkillFolder: "Выберите папку скиллов ИИ",
			AvatarTitle: "Выберите аватар", AvatarImages: "Изображения (*.png, *.jpg, *.jpeg, *.webp, *.gif)",
			CoverTitle: "Выберите обложку профиля", CoverImages: "Изображения (*.png, *.jpg, *.jpeg, *.webp)",
			ChatImageTitle: "Выберите изображение для чата", ChatImages: "Изображения (*.png, *.jpg, *.jpeg, *.webp, *.gif)",
			RollbackTitle:  "Typhon",
			RollbackAsk:    "Typhon не смог запуститься два раза подряд.\n\nВернуться на версию %s? Данные, изменённые после обновления, будут возвращены к состоянию до него.",
			RollbackFailed: "Не удалось вернуться на прежнюю версию: %v",
		}
	}
	return Labels{
		OpenTyphon: "Open Typhon", Quit: "Quit",
		RecentGames: "Recent games", Playing: "playing", StopGame: "Stop",
		LibraryTask: "Library", DownloadsTask: "Downloads",
		AllFiles: "All files", Executables: "Executable files (*.exe)",
		TorrentTitle: "Choose a torrent file", Torrents: "Torrent files (*.torrent)",
		FeedTitle: "Choose a source file", Feeds: "Source file (*.json)",
		TargetFolder: "Choose the destination folder",
		ThemeTitle:   "Choose a theme file", Themes: "Theme file (*.typhontheme, *.json)",
		ExportTheme: "Save theme", Theme: "Theme file (*.typhontheme)",
		SkillFolder: "Choose the AI skills folder",
		AvatarTitle: "Choose an avatar", AvatarImages: "Images (*.png, *.jpg, *.jpeg, *.webp, *.gif)",
		CoverTitle: "Choose a profile cover", CoverImages: "Images (*.png, *.jpg, *.jpeg, *.webp)",
		ChatImageTitle: "Choose an image for the chat", ChatImages: "Images (*.png, *.jpg, *.jpeg, *.webp, *.gif)",
		RollbackTitle:  "Typhon",
		RollbackAsk:    "Typhon failed to start twice in a row.\n\nGo back to version %s? Data changed after the update will be returned to how it was before it.",
		RollbackFailed: "Could not go back to the previous version: %v",
	}
}
