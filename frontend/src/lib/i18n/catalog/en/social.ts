import type { Message } from '../../types';
import type { SocialKey } from '../ru/social';

export const social: Record<SocialKey, Message> = {
  // lib/social/view.ts — relation labels/hints
  'social.relationNone': 'Add friend',
  'social.relationOutgoing': 'Request sent',
  'social.relationIncoming': 'Accept',
  'social.relationFriend': 'Friends',
  'social.relationSelf': 'This is you',
  'social.relationBlocked': 'Unavailable',
  'social.relationHintOutgoing': 'Request already sent',
  'social.relationHintIncoming': "This user already sent you a request — accept it on the Requests tab",
  'social.relationHintFriend': 'You are already friends',
  'social.relationHintBlocked': 'User unavailable',

  // lib/social/view.ts — commonGameLabel
  'social.userGameInstalledBoth': 'installed for both',
  'social.userGameInstallBoth': 'needs installing for both',
  'social.userGameInstallYou': 'you need to install it',
  'social.userGameInstallNamed': 'needs installing: {name}',
  'social.userGameInstallOther': 'they need to install it',

  // lib/social/view.ts — memberSince, reused in ProfileHeader.svelte
  'social.memberSince': 'Member since {date}',
  'social.memberSinceLabel': 'Member since',

  // shared friends title (notification title + PageHeader)
  'social.friendsTitle': 'Friends',

  // lib/social/feed.ts — reactions
  'social.reactionFire': 'Fire',
  'social.reactionSalute': 'Salute',
  'social.reactionHeart': 'Heart',
  'social.reactionClap': 'Applause',
  'social.reactionSkull': 'Skull',
  'social.reactionParty': 'Party',
  'social.reactionEyes': 'Eyes',
  'social.reactionJoy': 'Laugh',

  // lib/social/feed.ts — event kind labels
  'social.feedCompleted': 'Completed',
  'social.feedStarted': 'New game',
  'social.feedFavorited': 'Favorited',

  // lib/social/presence.ts
  'social.presenceOnline': 'Online',
  'social.presenceAway': 'Away',
  'social.presenceBusy': 'Do not disturb',
  'social.presenceInvisible': 'Invisible',
  'social.presenceOffline': 'Offline',
  'social.presenceJustNow': 'just now',
  'social.presenceMinutesAgo': '{count} min ago',
  'social.presenceHoursAgo': '{count}h ago',
  'social.presenceYesterdayLower': 'yesterday',
  'social.presenceDaysAgo': '{count}d ago',

  // shared "playing" phrases (presence.ts, profile/view.ts, several screens)
  'social.playing': 'Playing',
  'social.playingNamed': 'Playing: {name}',
  'social.playingLabel': 'Playing:',
  'social.noteAdd': 'Add note',
  'social.noteEdit': 'Edit note',
  'social.notePlaceholder': 'A few words about it',
  'social.noteSave': 'Save',
  'social.noteCancel': 'Cancel',
  'social.noteRemove': 'Remove note',
  'social.noteMore': 'More',
  'social.noteLess': 'Less',
  'social.noteLeft': 'Left: {count}',
  'social.playingIn': 'Playing {name}',

  // lib/social/openGame.ts
  'social.userOpenGameFailed': 'Could not open the game',

  // shared titles/buttons across user + profile screens
  'social.favoriteGamesTitle': 'Favorite games',
  'social.recentActivityTitle': 'Recent activity',
  'social.userCommonTitle': 'You both play',
  'social.userMutualTitle': 'Mutual friends ({count})',
  'social.viewAllButton': 'All',
  'social.nowPlayingTitle': 'Now playing',
  'social.aboutTitle': 'About',
  'social.recentlyPlayedTitle': 'Recently played',
  'social.profileLabel': 'Profile',
  'social.guestName': 'Guest',
  'social.copied': 'Copied',
  'social.copyFailed': 'Could not copy',
  'social.loadingEllipsis': 'Loading…',
  'social.moreLabel': 'More',
  'social.declineButton': 'Decline',
  'social.unfriendLabel': 'Remove friend',
  'social.blockLabel': 'Block',
  'social.cancelRequestButton': 'Cancel',
  'social.signInButton': 'Sign in',
  'social.createAccountButton': 'Create account',
  'social.signInFailed': 'Could not open sign in',
  'social.friendsNowFriends': 'You are now friends',
  'social.saving': 'Saving…',
  'social.saveFailed': 'Could not save',

  // routes/user/UserProfile.svelte
  'social.userGuestTitle': 'Profiles need an account',
  'social.userGuestDesc': "Sign in to see other players' profiles, games you have in common, and their friends.",
  'social.userNotFoundTitle': 'User not found',
  'social.userNotFoundDesc': 'Check the username or friend code',
  'social.userLoadFailed': 'Could not load the profile',
  'social.userRefreshFailed': 'Could not refresh the profile',
  'social.requestCancelled': 'Request cancelled',
  'social.requestDeclined': 'Request declined',
  'social.friendsUnfriended': 'Removed from friends',
  'social.userBlocked': 'User blocked',
  'social.actionFailed': 'Could not complete the action',
  'social.userProfileClosed': 'Profile is private',
  'social.userRestToFriends': 'The rest is visible to friends',

  // routes/user/UserStats.svelte
  'social.statsHiddenHint': 'The profile owner hid this data',
  'social.statGames': 'Games',
  'social.statHours': 'Hours',
  'social.statCompleted': 'Completed',
  'social.statsHiddenLabel': 'Stats hidden',

  // routes/profile/HiddenBadge.svelte
  'social.hiddenDefaultHint': "Hidden from others. You can see this block, they can't.",
  'social.hiddenLabel': 'Hidden',

  // routes/profile/ProfileHeader.svelte
  'social.editProfileLabel': 'Edit',
  'social.profileSettingsTitle': 'Profile settings',
  'social.signingOut': 'Signing out…',
  'social.signOutLabel': 'Sign out',
  'social.signOutFailed': 'Could not sign out',
  'social.profileUpdated': 'Profile updated',
  'social.editRequiresConnection': 'You can only edit your profile while connected to the server',
  'social.guestProfileHint': 'Sign in to keep your profile saved to your account',
  'social.hiddenStatusHint': "Hidden from others. You can see the status, they can't.",
  'social.onlineStatusHiddenHint': 'The "Online" status is hidden from others. You can see it.',
  'social.displayNameLabel': 'Display name',
  'social.usernameLabel': 'Username',
  'social.emailHint': "Email can't be changed yet, and nobody sees it except you",

  // routes/profile/ProfileActivity.svelte
  'social.playedFor': 'Played {value}',
  'social.viewAllActivity': 'View all activity',

  // routes/profile/ProfilePlaying.svelte
  'social.hiddenGenericHint': "Hidden from others. You can see this, they can't.",

  // routes/profile/ProfileSettingsModal.svelte
  'social.settingsSaved': 'Profile settings saved',
  'social.settingsRequireConnection': 'Profile settings can only change while connected to the server.',
  'social.whatOthersSee': 'What others see',
  'social.visibilityExplain': 'The access level sets who can see the profile at all, the toggles set exactly what.',
  'social.whoSeesProfile': 'Who sees the profile',
  'social.friendsSeeMore': 'Friends always see more than everyone else',
  'social.flagOnlineLabel': 'Online status',
  'social.flagOnlineSub': "Others see that you're in the launcher",
  'social.flagPlayingLabel': "What I'm playing",
  'social.flagPlayingSub': 'Current game and the "Now playing" list',
  'social.flagLibraryLabel': 'Library',
  'social.flagLibrarySub': 'Game list, games in common, and "friends played" on the game page',
  'social.flagPlaytimeLabel': 'Playtime',
  'social.flagPlaytimeSub': 'Hours on the profile and next to games',
  'social.flagActivitySub': 'Games played by day, without launch times',
  'social.flagStatsLabel': 'Stats',
  'social.flagStatsSub': 'Games, hours, completed, currently playing',
  'social.showcaseHeading': 'Showcase',
  'social.showcaseExplain': 'Up to three blocks, in the order you choose.',
  'social.moveUp': 'Move up: {title}',
  'social.moveDown': 'Move down: {title}',
  'social.removeButton': 'Remove',

  // routes/profile/ProfileShowcase.svelte
  'social.manageFavorites': 'Manage favorites',
  'social.completedOn': 'Completed {date}',

  // routes/profile/ProfileStats.svelte
  'social.statsHiddenFromOthers': 'Stats hidden from others. You can see them.',

  // routes/friends/Friends.svelte
  'social.friendsRefreshFailed': 'Could not refresh the friends list',
  'social.friendsBlockedLoadFailed': 'Could not load the blocked list',
  'social.friendsUnfriendFailed': 'Could not remove from friends',
  'social.blockFailed': 'Could not block the user',
  'social.friendsSortName': 'Name (A-Z)',
  'social.friendsSortStatus': 'By status',
  'social.friendsEmptyOnlineTitle': 'Nobody online',
  'social.friendsEmptyOnlineDesc': "None of your friends are online right now.",
  'social.friendsEmptyAwayTitle': 'Nobody away',
  'social.friendsEmptyAwayDesc': "None of your friends stepped away right now.",
  'social.friendsEmptyOfflineTitle': 'Everyone is around',
  'social.friendsEmptyOfflineDesc': 'All your friends are online or away right now.',
  'social.friendsTabAll': 'All friends',
  'social.friendsTabAway': 'Away',
  'social.friendsTabRequests': 'Requests',
  'social.friendsTabBlocked': 'Blocked',
  'social.friendsSubtitle': 'Requests, friend list and blocked users',
  'social.friendsMyCodeButton': 'My code',
  'social.friendsAddFriend': 'Add friend',
  'social.friendsGuestTitle': 'Friends need an account',
  'social.friendsGuestDesc': 'Sign in to add friends, see their profiles and games you have in common.',
  'social.friendsConsentTitle': 'Account sync is needed',
  'social.friendsConsentDesc':
    "Friends run on top of sync: without it, the server has nothing to show your friends, or you their profiles.",
  'social.friendsEnableSync': 'Enable sync',
  'social.friendsSearchFriendsPlaceholder': 'Search friends',
  'social.friendsSortLabel': 'Sort: {value}',
  'social.friendsViewList': 'List',
  'social.friendsViewGrid': 'Grid',
  'social.friendsEmptyNobodyTitle': 'Nobody yet',
  'social.friendsEmptyNobodyDesc': 'Add a friend by username or by code.',
  'social.friendsEmptySearchTitle': 'Nothing found',
  'social.friendsEmptySearchDesc': 'Try a different search.',
  'social.friendsEmptyRequestsTitle': 'No requests',
  'social.friendsEmptyRequestsDesc': 'New requests will appear here.',
  'social.friendsIncomingHeading': 'Incoming requests',
  'social.acceptRequestFailed': 'Could not accept the request',
  'social.declineRequestFailed': 'Could not decline the request',
  'social.friendsSentHeading': 'Sent',
  'social.cancelRequestFailed': 'Could not cancel the request',
  'social.friendsSafetyTitle': 'Your safety is our priority',
  'social.friendsSafetyDesc': "Don't accept requests from strangers, and don't share personal data.",
  'social.friendsEmptyBlockedTitle': 'Nobody blocked',
  'social.friendsEmptyBlockedDesc': "Blocked users can't see your profile or send a request.",
  'social.unblockButton': 'Unblock',
  'social.unblockFailed': 'Could not unblock',
  'social.userUnblocked': 'User unblocked',

  // routes/friends/AddFriendModal.svelte
  'social.friendsFindUserFailed': 'Could not find the user',
  'social.friendsSendRequestFailed': 'Could not send the request',
  'social.friendsSearchPlaceholder': '@username or code TY-XXXX-XXXX',
  'social.friendsSearchHint': 'Search by username or by a code someone shared with you.',
  'social.friendsSendRequest': 'Send request',
  'social.friendsOpenProfile': 'Open profile',

  // routes/friends/FriendCodeCard.svelte
  'social.friendsCodeFetchFailed': 'Could not get the code',
  'social.friendsCodeRotated': 'New code ready',
  'social.friendsCodeRotateFailed': 'Could not change the code',
  'social.friendsCopyCode': 'Copy code',
  'social.friendsYourCode': 'Your code',
  'social.friendsGenerateNew': 'Generate new',
  'social.friendsRotating': 'Changing…',
  'social.friendsChangeCodeTitle': 'Change code',
  'social.friendsChangeCodeWarning':
    "The old code will stop working: nobody will be able to find you with it anymore. Already-sent requests and your friend list won't be affected.",
};
