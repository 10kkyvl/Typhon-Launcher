package wrappers

import "typhon/internal/uierr"

var (
	ErrUnsupported     = uierr.New("wrappers.unsupported", "graphics wrappers can only be installed on Windows")
	ErrBadWrapper      = uierr.New("wrappers.bad_wrapper", "unknown wrapper")
	ErrBadPreset       = uierr.New("wrappers.bad_preset", "unknown preset for this wrapper")
	ErrBadGameID       = uierr.New("wrappers.bad_game_id", "invalid game id")
	ErrRunning         = uierr.New("wrappers.running", "the game is running")
	ErrNoInstallDir    = uierr.New("wrappers.no_install_dir", "the game has no install directory or executable")
	ErrOutsideDir      = uierr.New("wrappers.outside_install_dir", "the executable is outside the install directory")
	ErrNotPE           = uierr.New("wrappers.not_pe", "the executable is not a Windows PE file")
	ErrNoTarget        = uierr.New("wrappers.no_target", "the executable imports nothing this wrapper can replace")
	ErrArchUnsupported = uierr.New("wrappers.arch_unsupported", "dgVoodoo2 ships DirectDraw, Direct3D 8 and Glide for 32-bit games only")
	ErrActive          = uierr.New("wrappers.already_active", "a wrapper is already installed for this game")
	ErrNotActive       = uierr.New("wrappers.not_active", "no wrapper is installed for this game")
	ErrTampered        = uierr.New("wrappers.tampered", "a file was changed after the wrapper was installed")
	ErrSymlink         = uierr.New("wrappers.symlink", "the target path is a symlink or not a regular file")
	ErrBackupCorrupt   = uierr.New("wrappers.backup_corrupt", "the backup of an original file does not match its recorded hash")
	ErrArchiveHash     = uierr.New("wrappers.archive_hash", "downloaded archive hash does not match the pinned one")
	ErrBadArchive      = uierr.New("wrappers.bad_archive", "the archive is malformed or unsafe")
	ErrDownload        = uierr.New("wrappers.download", "download failed")
	ErrForeignHost     = uierr.New("wrappers.foreign_host", "download redirected to a host that is not allowed")
	ErrNeedsRepair     = uierr.New("wrappers.needs_repair", "an interrupted operation must be finished with Disable first")
	ErrBusy            = uierr.New("wrappers.busy", "another wrappers operation on this game is in progress")
	ErrGameDirMissing  = uierr.New("wrappers.game_dir_missing", "the game directory is not available")
	ErrBadState        = uierr.New("wrappers.bad_state", "the wrappers state file is unreadable")
)
