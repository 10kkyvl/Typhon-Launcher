package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const portableInstanceHexLen = 12

func portableInstanceID(configDir string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(configDir)))
	return singleInstanceID + "-" + hex.EncodeToString(sum[:])[:portableInstanceHexLen]
}
