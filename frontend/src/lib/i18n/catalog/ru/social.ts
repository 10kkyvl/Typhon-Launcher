export const social = {
  // lib/social/view.ts — relation labels/hints
  'social.relationNone': 'Добавить в друзья',
  'social.relationOutgoing': 'Заявка отправлена',
  'social.relationIncoming': 'Принять',
  'social.relationFriend': 'В друзьях',
  'social.relationSelf': 'Это вы',
  'social.relationBlocked': 'Недоступен',
  'social.relationHintOutgoing': 'Заявка уже отправлена',
  'social.relationHintIncoming': 'Этот пользователь уже отправил вам заявку — примите её во вкладке «Заявки»',
  'social.relationHintFriend': 'Вы уже друзья',
  'social.relationHintBlocked': 'Пользователь недоступен',

  // lib/social/view.ts — commonGameLabel
  'social.userGameInstalledBoth': 'установлена у обоих',
  'social.userGameInstallBoth': 'нужно установить обоим',
  'social.userGameInstallYou': 'нужно установить вам',
  'social.userGameInstallNamed': 'нужно установить: {name}',
  'social.userGameInstallOther': 'нужно установить ему',

  // lib/social/view.ts — memberSince, reused in ProfileHeader.svelte
  'social.memberSince': 'Участник с {date}',
  'social.memberSinceLabel': 'Участник с',

  // shared friends title (notification title + PageHeader)
  'social.friendsTitle': 'Друзья',

  // lib/social/feed.ts — reactions
  'social.reactionFire': 'Огонь',
  'social.reactionSalute': 'Салют',
  'social.reactionHeart': 'Сердце',
  'social.reactionClap': 'Аплодисменты',
  'social.reactionSkull': 'Череп',
  'social.reactionParty': 'Праздник',
  'social.reactionEyes': 'Глаза',
  'social.reactionJoy': 'Смех',

  // lib/social/feed.ts — event kind labels
  'social.feedCompleted': 'Пройдена',
  'social.feedStarted': 'Новая игра',
  'social.feedFavorited': 'В любимых',

  // lib/social/presence.ts
  'social.presenceOnline': 'В сети',
  'social.presenceAway': 'Отошёл',
  'social.presenceBusy': 'Не беспокоить',
  'social.presenceInvisible': 'Невидимка',
  'social.presenceOffline': 'Не в сети',
  'social.presenceJustNow': 'только что',
  'social.presenceMinutesAgo': '{count} мин назад',
  'social.presenceHoursAgo': '{count} ч назад',
  'social.presenceYesterdayLower': 'вчера',
  'social.presenceDaysAgo': '{count} дн. назад',

  // shared "playing" phrases (presence.ts, profile/view.ts, several screens)
  'social.playing': 'Играет',
  'social.playingNamed': 'Играет: {name}',
  'social.playingLabel': 'Играет:',
  'social.playingIn': 'Играет в {name}',

  // lib/social/openGame.ts
  'social.userOpenGameFailed': 'Не удалось открыть игру',

  // shared titles/buttons across user + profile screens
  'social.favoriteGamesTitle': 'Любимые игры',
  'social.recentActivityTitle': 'Недавняя активность',
  'social.userCommonTitle': 'Играете оба',
  'social.userMutualTitle': 'Общие друзья ({count})',
  'social.viewAllButton': 'Все',
  'social.nowPlayingTitle': 'Сейчас играет',
  'social.aboutTitle': 'О себе',
  'social.recentlyPlayedTitle': 'Недавно играл',
  'social.profileLabel': 'Профиль',
  'social.guestName': 'Гость',
  'social.copied': 'Скопировано',
  'social.copyFailed': 'Не удалось скопировать',
  'social.loadingEllipsis': 'Загрузка…',
  'social.moreLabel': 'Ещё',
  'social.declineButton': 'Отклонить',
  'social.unfriendLabel': 'Удалить из друзей',
  'social.blockLabel': 'Заблокировать',
  'social.cancelRequestButton': 'Отменить',
  'social.signInButton': 'Войти',
  'social.createAccountButton': 'Создать аккаунт',
  'social.signInFailed': 'Не удалось открыть вход',
  'social.friendsNowFriends': 'Вы теперь друзья',
  'social.saving': 'Сохранение…',
  'social.saveFailed': 'Не удалось сохранить',

  // routes/user/UserProfile.svelte
  'social.userGuestTitle': 'Профили доступны с аккаунтом',
  'social.userGuestDesc': 'Войдите, чтобы смотреть профили других игроков, их общие с вами игры и друзей.',
  'social.userNotFoundTitle': 'Пользователь не найден',
  'social.userNotFoundDesc': 'Проверьте имя пользователя или код друга',
  'social.userLoadFailed': 'Не удалось загрузить профиль',
  'social.userRefreshFailed': 'Не удалось обновить профиль',
  'social.requestCancelled': 'Заявка отменена',
  'social.requestDeclined': 'Заявка отклонена',
  'social.friendsUnfriended': 'Удалён из друзей',
  'social.userBlocked': 'Пользователь заблокирован',
  'social.actionFailed': 'Не удалось выполнить действие',
  'social.userProfileClosed': 'Профиль закрыт',
  'social.userRestToFriends': 'Остальное видно друзьям',

  // routes/user/UserStats.svelte
  'social.statsHiddenHint': 'Владелец профиля скрыл эти данные',
  'social.statGames': 'Игр',
  'social.statHours': 'Часов',
  'social.statCompleted': 'Пройдено',
  'social.statsHiddenLabel': 'Статистика скрыта',

  // routes/profile/HiddenBadge.svelte
  'social.hiddenDefaultHint': 'Скрыто от других. Вы видите этот блок, остальные — нет.',
  'social.hiddenLabel': 'Скрыто',

  // routes/profile/ProfileHeader.svelte
  'social.editProfileLabel': 'Редактировать',
  'social.profileSettingsTitle': 'Настройки профиля',
  'social.signingOut': 'Выход…',
  'social.signOutLabel': 'Выйти',
  'social.signOutFailed': 'Не удалось выйти',
  'social.profileUpdated': 'Профиль обновлён',
  'social.editRequiresConnection': 'Изменить профиль можно только при связи с сервером',
  'social.guestProfileHint': 'Войдите, чтобы профиль сохранялся в аккаунте',
  'social.friendCodeInline': 'Код друга: {code}',
  'social.copyFriendCodeLabel': 'Скопировать код друга',
  'social.hiddenStatusHint': 'Скрыто от других. Вы видите статус, остальные — нет.',
  'social.onlineStatusHiddenHint': 'Статус «В сети» скрыт от других. Вы его видите.',
  'social.displayNameLabel': 'Отображаемое имя',
  'social.usernameLabel': 'Имя пользователя',
  'social.emailHint': 'Email пока нельзя изменить и его не видит никто, кроме вас',

  // routes/profile/ProfileActivity.svelte
  'social.playedFor': 'Сыграно {value}',
  'social.viewAllActivity': 'Смотреть всю активность',

  // routes/profile/ProfilePlaying.svelte
  'social.hiddenGenericHint': 'Скрыто от других. Вы видите это, остальные — нет.',

  // routes/profile/ProfileSettingsModal.svelte
  'social.settingsSaved': 'Настройки профиля сохранены',
  'social.settingsRequireConnection': 'Настройки профиля меняются только при связи с сервером.',
  'social.whatOthersSee': 'Что видят другие',
  'social.visibilityExplain': 'Уровень доступа задаёт, кто вообще видит профиль, переключатели — что именно.',
  'social.whoSeesProfile': 'Кто видит профиль',
  'social.friendsSeeMore': 'Друзьям всегда видно больше, чем остальным',
  'social.flagOnlineLabel': 'Статус «В сети»',
  'social.flagOnlineSub': 'Другие видят, что вы в лаунчере',
  'social.flagPlayingLabel': 'Во что играю',
  'social.flagPlayingSub': 'Текущая игра и список «Сейчас играю»',
  'social.flagLibraryLabel': 'Библиотека',
  'social.flagLibrarySub': 'Список игр, общие игры и «друзья играли» на странице игры',
  'social.flagPlaytimeLabel': 'Наигранное время',
  'social.flagPlaytimeSub': 'Часы в профиле и рядом с играми',
  'social.flagActivitySub': 'Сыгранные игры по дням, без времени запуска',
  'social.flagStatsLabel': 'Статистика',
  'social.flagStatsSub': 'Игры, часы, пройдено, играю сейчас',
  'social.showcaseHeading': 'Витрина',
  'social.showcaseExplain': 'До трёх блоков, в выбранном порядке.',
  'social.moveUp': 'Выше: {title}',
  'social.moveDown': 'Ниже: {title}',
  'social.removeButton': 'Убрать',

  // routes/profile/ProfileShowcase.svelte
  'social.manageFavorites': 'Управлять избранным',
  'social.completedOn': 'Пройдена {date}',

  // routes/profile/ProfileStats.svelte
  'social.statsHiddenFromOthers': 'Статистика скрыта от других. Вы её видите.',

  // routes/friends/Friends.svelte
  'social.friendsRefreshFailed': 'Не удалось обновить список друзей',
  'social.friendsBlockedLoadFailed': 'Не удалось загрузить список заблокированных',
  'social.friendsUnfriendFailed': 'Не удалось удалить из друзей',
  'social.blockFailed': 'Не удалось заблокировать',
  'social.friendsSortName': 'Имя (А-Я)',
  'social.friendsSortStatus': 'По статусу',
  'social.friendsEmptyOnlineTitle': 'Никто не в сети',
  'social.friendsEmptyOnlineDesc': 'Сейчас никто из друзей не в сети.',
  'social.friendsEmptyAwayTitle': 'Никто не отошёл',
  'social.friendsEmptyAwayDesc': 'Сейчас никто из друзей не отходил.',
  'social.friendsEmptyOfflineTitle': 'Все на связи',
  'social.friendsEmptyOfflineDesc': 'Все друзья сейчас в сети или отошли.',
  'social.friendsTabAll': 'Все друзья',
  'social.friendsTabAway': 'Отошли',
  'social.friendsTabRequests': 'Заявки',
  'social.friendsTabBlocked': 'Заблокированные',
  'social.friendsSubtitle': 'Заявки, список друзей и заблокированные',
  'social.friendsMyCodeButton': 'Мой код',
  'social.friendsAddFriend': 'Добавить друга',
  'social.friendsGuestTitle': 'Друзья доступны с аккаунтом',
  'social.friendsGuestDesc': 'Войдите, чтобы добавлять друзей, видеть их профили и общие игры.',
  'social.friendsConsentTitle': 'Нужна синхронизация с аккаунтом',
  'social.friendsConsentDesc':
    'Друзья работают поверх синхронизации: без неё серверу нечего показать вашим друзьям, а вам — их профили.',
  'social.friendsEnableSync': 'Включить синхронизацию',
  'social.friendsSearchFriendsPlaceholder': 'Поиск друзей',
  'social.friendsSortLabel': 'Сортировка: {value}',
  'social.friendsViewList': 'Список',
  'social.friendsViewGrid': 'Сетка',
  'social.friendsEmptyNobodyTitle': 'Пока никого',
  'social.friendsEmptyNobodyDesc': 'Добавьте друга по имени пользователя или по коду.',
  'social.friendsEmptySearchTitle': 'Ничего не найдено',
  'social.friendsEmptySearchDesc': 'Попробуйте изменить запрос поиска.',
  'social.friendsEmptyRequestsTitle': 'Заявок нет',
  'social.friendsEmptyRequestsDesc': 'Новые заявки появятся здесь.',
  'social.friendsIncomingHeading': 'Входящие заявки',
  'social.acceptRequestFailed': 'Не удалось принять заявку',
  'social.declineRequestFailed': 'Не удалось отклонить заявку',
  'social.friendsSentHeading': 'Отправленные',
  'social.cancelRequestFailed': 'Не удалось отменить заявку',
  'social.friendsSafetyTitle': 'Ваша безопасность — наш приоритет',
  'social.friendsSafetyDesc': 'Не принимайте заявки от незнакомых пользователей и не делитесь личными данными.',
  'social.friendsEmptyBlockedTitle': 'Никого не заблокировано',
  'social.friendsEmptyBlockedDesc': 'Заблокированные не видят ваш профиль и не могут отправить заявку.',
  'social.unblockButton': 'Разблокировать',
  'social.unblockFailed': 'Не удалось разблокировать',
  'social.userUnblocked': 'Пользователь разблокирован',

  // routes/friends/AddFriendModal.svelte
  'social.friendsFindUserFailed': 'Не удалось найти пользователя',
  'social.friendsSendRequestFailed': 'Не удалось отправить заявку',
  'social.friendsSearchPlaceholder': '@имя или код TY-XXXX-XXXX',
  'social.friendsSearchHint': 'Найдите по имени пользователя или по коду, которым с вами поделились.',
  'social.friendsSendRequest': 'Отправить заявку',
  'social.friendsOpenProfile': 'Открыть профиль',

  // routes/friends/FriendCodeCard.svelte
  'social.friendsCodeFetchFailed': 'Не удалось получить код',
  'social.friendsCodeRotated': 'Новый код готов',
  'social.friendsCodeRotateFailed': 'Не удалось сменить код',
  'social.friendsMyCodeTitle': 'Мой код друга',
  'social.friendsShareHint': 'Поделитесь кодом с друзьями, чтобы они добавили вас в Typhon.',
  'social.friendsCopyCode': 'Скопировать код',
  'social.friendsYourCode': 'Ваш код',
  'social.friendsGenerateNew': 'Сгенерировать новый',
  'social.friendsRotating': 'Смена…',
  'social.friendsChangeCodeTitle': 'Сменить код',
  'social.friendsChangeCodeWarning':
    'Старый код перестанет работать: по нему вас больше никто не найдёт. Уже отправленные заявки и список друзей это не затронет.',
} as const;

export type SocialKey = keyof typeof social;
