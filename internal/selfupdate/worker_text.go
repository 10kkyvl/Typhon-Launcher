package selfupdate

type workerLabels struct {
	baseTitle, versionPrefix, waiting, installing, restarting    string
	failed, parentRunning, restoring, launchFailed, openManually string
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
	}
}

func (text workerLabels) title(version string) string {
	if version == "" {
		return text.baseTitle
	}
	return text.versionPrefix + version
}
