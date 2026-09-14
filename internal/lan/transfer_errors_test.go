package lan

import (
	"strings"
	"testing"
)

func TestReceivedExecutableFailuresHaveLocalizableReasons(t *testing.T) {
	for _, test := range []struct{ exe, code string }{
		{"../outside.exe", "lan.exe_outside_install"},
		{"missing.exe", "lan.executable_missing"},
	} {
		t.Run(test.code, func(t *testing.T) {
			state := &transferState{transfer: Transfer{ID: "transfer", Status: TransferReceiving}}
			run := &runState{transfers: map[string]*transferState{"transfer": state}}
			service := &Service{}
			service.completeTransfer(run, "transfer", t.TempDir(), Offer{Exe: test.exe}, nil)
			if state.transfer.Status != TransferFailed || !strings.Contains(state.transfer.Error, "typhon:"+test.code) {
				t.Fatalf("failed transfer must retain a translatable error: %+v", state.transfer)
			}
		})
	}
}
