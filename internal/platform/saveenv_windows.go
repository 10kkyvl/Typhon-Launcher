package platform

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func SaveEnvs() ([]SaveEnv, error) {
	folders := []struct {
		token string
		id    *windows.KNOWNFOLDERID
	}{
		{TokenUserProfile, windows.FOLDERID_Profile},
		{TokenDocuments, windows.FOLDERID_Documents},
		{TokenAppData, windows.FOLDERID_RoamingAppData},
		{TokenLocalAppData, windows.FOLDERID_LocalAppData},
		{TokenLocalAppDataLow, windows.FOLDERID_LocalAppDataLow},
		{TokenSavedGames, windows.FOLDERID_SavedGames},
		{TokenPublic, windows.FOLDERID_Public},
		{TokenProgramData, windows.FOLDERID_ProgramData},
	}
	env := SaveEnv{}
	for _, f := range folders {
		path, err := knownFolder(f.id)
		if err != nil {
			return nil, fmt.Errorf("known folder {%s}: %w", f.token, err)
		}
		env[f.token] = path
	}
	return []SaveEnv{env}, nil
}
