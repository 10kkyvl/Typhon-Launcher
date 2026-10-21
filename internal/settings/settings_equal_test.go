package settings

import "reflect"

func sameSettings(a, b Settings) bool {
	return reflect.DeepEqual(a, b)
}
