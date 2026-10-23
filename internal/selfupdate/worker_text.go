package selfupdate

type workerLabels struct {
	baseTitle, versionPrefix, waiting, installing, restarting    string
	failed, parentRunning, restoring, launchFailed, openManually string
	rollbackPrefix, rollingBack, rollbackRestarting              string
	rollbackFailed, rollbackKept                                 string
}

func workerText(language string) workerLabels {
	// Older persisted specs have no language and were presented in Russian.
	if language == "ru" || language == "" {
		return workerLabels{
			baseTitle: "Обновление Typhon", versionPrefix: "Обновление Typhon до ",
			waiting:    "Ожидание закрытия лаунчера…",
			installing: "Устанавливаем новую версию, лаунчер запустится сам.",
			restarting: "Обновление установлено, запускаем Typhon…",
			failed:     "Не удалось обновить Typhon", parentRunning: "Лаунчер не закрылся, обновление отменено.",
			restoring:    "Возвращаем прежнюю версию. Подробности — в лаунчере.",
			launchFailed: "Не удалось запустить Typhon", openManually: "Лаунчер не запустился автоматически. Откройте Typhon вручную.",
			rollbackPrefix:     "Возврат Typhon на версию ",
			rollingBack:        "Возвращаем прежнюю версию и данные, лаунчер запустится сам.",
			rollbackRestarting: "Версия возвращена, запускаем Typhon…",
			rollbackFailed:     "Не удалось вернуть прежнюю версию Typhon",
			rollbackKept:       "Версия не изменилась. Подробности — в лаунчере.",
		}
	}
	return workerLabels{
		baseTitle: "Updating Typhon", versionPrefix: "Updating Typhon to ",
		waiting:    "Waiting for the launcher to close…",
		installing: "Installing the new version. The launcher will restart automatically.",
		restarting: "Update installed. Starting Typhon…",
		failed:     "Could not update Typhon", parentRunning: "The launcher did not close. The update was cancelled.",
		restoring:    "Restoring the previous version. Details will be available in the launcher.",
		launchFailed: "Could not start Typhon", openManually: "The launcher did not restart automatically. Open Typhon manually.",
		rollbackPrefix:     "Rolling Typhon back to ",
		rollingBack:        "Restoring the previous version and its data. The launcher will restart automatically.",
		rollbackRestarting: "Version restored. Starting Typhon…",
		rollbackFailed:     "Could not roll Typhon back",
		rollbackKept:       "The version was not changed. Details will be available in the launcher.",
	}
}

func (text workerLabels) title(version string) string {
	if version == "" {
		return text.baseTitle
	}
	return text.versionPrefix + version
}

func (text workerLabels) rollbackTitle(version string) string {
	return text.rollbackPrefix + version
}
